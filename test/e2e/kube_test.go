//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	sigsyaml "sigs.k8s.io/yaml"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

// apply server-side-applies every object of a multi-document YAML, so reruns are idempotent.
func (e *Env) apply(ctx context.Context, manifest []byte) error {
	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(manifest), 4096)
	for {
		obj := &unstructured.Unstructured{}
		if err := dec.Decode(&obj.Object); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if len(obj.Object) == 0 {
			continue
		}
		if err := e.Kube.Patch(ctx, obj, client.Apply, client.FieldOwner("frp-operator-e2e"), client.ForceOwnership); err != nil {
			return fmt.Errorf("apply %s %s/%s: %w", obj.GetKind(), obj.GetNamespace(), obj.GetName(), err)
		}
	}
}

// upsert creates obj, or replaces it if it already exists.
func (e *Env) upsert(ctx context.Context, obj client.Object) error {
	err := e.Kube.Create(ctx, obj)
	if !apierrors.IsAlreadyExists(err) {
		return err
	}
	existing := obj.DeepCopyObject().(client.Object)
	if err := e.Kube.Get(ctx, client.ObjectKeyFromObject(obj), existing); err != nil {
		return err
	}
	obj.SetResourceVersion(existing.GetResourceVersion())
	return e.Kube.Update(ctx, obj)
}

// ensureSecret creates secret, or updates it when its data differs, and reports whether an
// existing Secret changed (pods that read it only at startup then need a restart).
func (e *Env) ensureSecret(ctx context.Context, secret *corev1.Secret) (bool, error) {
	existing := &corev1.Secret{}
	err := e.Kube.Get(ctx, client.ObjectKeyFromObject(secret), existing)
	if apierrors.IsNotFound(err) {
		return false, e.Kube.Create(ctx, secret)
	}
	if err != nil {
		return false, err
	}
	if reflect.DeepEqual(existing.Data, secret.Data) {
		return false, nil
	}
	existing.Data = secret.Data
	return true, e.Kube.Update(ctx, existing)
}

// restartDeployment rolls a Deployment's pods and waits until only new, ready pods remain.
func (e *Env) restartDeployment(ctx context.Context, namespace, name string) error {
	deploy := &appsv1.Deployment{}
	if err := e.Kube.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, deploy); err != nil {
		return err
	}
	if deploy.Spec.Template.Annotations == nil {
		deploy.Spec.Template.Annotations = map[string]string{}
	}
	deploy.Spec.Template.Annotations["frp-operator-e2e/restartedAt"] = time.Now().Format(time.RFC3339Nano)
	if err := e.Kube.Update(ctx, deploy); err != nil {
		return err
	}
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		d := &appsv1.Deployment{}
		if err := e.Kube.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, d); err != nil {
			return false, nil
		}
		want := *d.Spec.Replicas
		return d.Status.ObservedGeneration >= d.Generation && d.Status.UpdatedReplicas == want &&
			d.Status.AvailableReplicas == want && d.Status.Replicas == want, nil
	})
}

// deleteAndWait deletes obj (if present) and waits until it is gone, finalizers included.
func (e *Env) deleteAndWait(ctx context.Context, obj client.Object) error {
	if err := e.Kube.Delete(ctx, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		probe := obj.DeepCopyObject().(client.Object)
		return apierrors.IsNotFound(e.Kube.Get(ctx, client.ObjectKeyFromObject(obj), probe)), nil
	})
}

func (e *Env) waitPodsReady(ctx context.Context, namespace string, timeout time.Duration) error {
	problem := "no pods yet"
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		pods := &corev1.PodList{}
		if err := e.Kube.List(ctx, pods, client.InNamespace(namespace)); err != nil {
			problem = err.Error()
			return false, nil
		}
		if len(pods.Items) == 0 {
			problem = "no pods yet"
			return false, nil
		}
		for i := range pods.Items {
			if pods.Items[i].DeletionTimestamp != nil {
				continue // a pod on its way out after a restart
			}
			if !podReady(&pods.Items[i]) {
				problem = podProblem(&pods.Items[i])
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("namespace %s not ready after %s: %s", namespace, timeout, problem)
	}
	return nil
}

func (e *Env) podIP(ctx context.Context, namespace, selector string) (string, error) {
	pods, err := e.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", err
	}
	for i := range pods.Items {
		if pods.Items[i].DeletionTimestamp == nil && podReady(&pods.Items[i]) && pods.Items[i].Status.PodIP != "" {
			return pods.Items[i].Status.PodIP, nil
		}
	}
	return "", fmt.Errorf("no ready pod %q in %s", selector, namespace)
}

// exec runs cmd in a pod container and returns stdout; a non-zero exit is an error with stderr.
func (e *Env) exec(ctx context.Context, namespace, pod, container string, cmd ...string) (string, error) {
	req := e.Clientset.CoreV1().RESTClient().Post().Resource("pods").Name(pod).Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{Container: container, Command: cmd, Stdout: true, Stderr: true}, clientgoscheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(e.Config, "POST", req.URL())
	if err != nil {
		return "", err
	}
	var stdout, stderr bytes.Buffer
	if err := executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: &stdout, Stderr: &stderr}); err != nil {
		return stdout.String(), fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// sh runs a shell script on the test client pod (netshoot in e2e-client).
func (e *Env) sh(ctx context.Context, script string) (string, error) {
	return e.exec(ctx, clientNamespace, "client", "client", "sh", "-c", script)
}

func (e *Env) logs(ctx context.Context, namespace, pod, container string) (string, error) {
	out, err := e.Clientset.CoreV1().Pods(namespace).GetLogs(pod, &corev1.PodLogOptions{Container: container}).DoRaw(ctx)
	return string(out), err
}

func (e *Env) dumpPodLogs(ctx context.Context, namespace, dir string) {
	pods, err := e.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}
	_ = os.MkdirAll(dir, 0o755)
	for _, p := range pods.Items {
		for _, c := range p.Spec.Containers {
			out, err := e.logs(ctx, namespace, p.Name, c.Name)
			if err != nil {
				out = err.Error()
			}
			_ = os.WriteFile(filepath.Join(dir, p.Name+"_"+c.Name+".log"), []byte(out), 0o644)
		}
	}
}

func (e *Env) dumpEvents(ctx context.Context, namespace, file string) {
	events, err := e.Clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}
	var b strings.Builder
	for _, ev := range events.Items {
		fmt.Fprintf(&b, "%s %s %s/%s %s %s: %s\n", ev.LastTimestamp.Format(time.RFC3339), ev.Namespace,
			ev.InvolvedObject.Kind, ev.InvolvedObject.Name, ev.Type, ev.Reason, ev.Message)
	}
	_ = os.MkdirAll(filepath.Dir(file), 0o755)
	_ = os.WriteFile(file, []byte(b.String()), 0o644)
}

// dumpCase saves a failed case: pod logs, events, Client status YAML and rendered frpc configs.
func (e *Env) dumpCase(ctx context.Context, namespace string) {
	if e.Artifacts == "" {
		return
	}
	dir := filepath.Join(e.Artifacts, namespace)
	e.dumpPodLogs(ctx, namespace, dir)
	e.dumpEvents(ctx, namespace, filepath.Join(dir, "events.txt"))
	clients := &frpv1alpha1.ClientList{}
	if err := e.Kube.List(ctx, clients, client.InNamespace(namespace)); err == nil {
		for _, cl := range clients.Items {
			if out, err := sigsyaml.Marshal(cl); err == nil {
				_ = os.WriteFile(filepath.Join(dir, "client_"+cl.Name+".yaml"), out, 0o644)
			}
		}
	}
	configMaps := &corev1.ConfigMapList{}
	if err := e.Kube.List(ctx, configMaps, client.InNamespace(namespace)); err == nil {
		for _, cm := range configMaps.Items {
			if data, ok := cm.Data["config.toml"]; ok {
				_ = os.WriteFile(filepath.Join(dir, cm.Name+".toml"), []byte(data), 0o644)
			}
		}
	}
}
