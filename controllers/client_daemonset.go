/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/builder"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/status"
)

// Annotations stamped on the DaemonSet pod template to detect drift without
// comparing against the live, server-defaulted PodSpec. configHashAnnotation
// fingerprints the rendered ConfigMap and triggers a rolling restart when
// the frpc TOML changes; specHashAnnotation fingerprints the rendered
// PodTemplateSpec so image / podTemplate edits are picked up without
// chasing server-side defaults (e.g. dnsPolicy, terminationGracePeriodSeconds,
// resource quotas) that would otherwise make a reflect.DeepEqual diverge
// forever and force an infinite update loop.
const (
	configHashAnnotation    = "frp.zufardhiyaulhaq.com/config-hash"
	specHashAnnotation      = "frp.zufardhiyaulhaq.com/spec-hash"
	reloadPendingAnnotation = "frp.zufardhiyaulhaq.com/reload-pending"
)

// reconcileDaemonSet handles the Client.spec.workload.kind == DaemonSet path.
// It mirrors the bare-Pod reconcile flow but emits an appsv1.DaemonSet and
// drives config rollouts via a template annotation hash rather than the
// admin-API reload used for the single-pod case.
func (r *ClientReconciler) reconcileDaemonSet(
	ctx context.Context,
	client *frpv1alpha1.Client,
	configmap *corev1.ConfigMap,
	createdConfigMap *corev1.ConfigMap,
	upstreamCount, visitorCount int,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.Info("Build daemonset")
	dsBuilder := builder.NewDaemonSetBuilder().
		SetName(client.Name).
		SetNamespace(client.Namespace).
		SetImage("fatedier/frpc:v0.65.0").
		SetPodTemplate(client.Spec.PodTemplate)

	if client.Spec.Server.TLS != nil {
		if client.Spec.Server.TLS.CertFile != nil {
			dsBuilder.SetTLSSecret(client.Spec.Server.TLS.CertFile.Secret.Name)
		} else if client.Spec.Server.TLS.KeyFile != nil {
			dsBuilder.SetTLSSecret(client.Spec.Server.TLS.KeyFile.Secret.Name)
		}
		if client.Spec.Server.TLS.TrustedCAFile != nil {
			if client.Spec.Server.TLS.TrustedCAFile.ConfigMap != nil {
				dsBuilder.SetTLSCAConfigMap(client.Spec.Server.TLS.TrustedCAFile.ConfigMap.Name)
			} else if client.Spec.Server.TLS.TrustedCAFile.Secret != nil && dsBuilder.TLSSecret == "" {
				dsBuilder.SetTLSSecret(client.Spec.Server.TLS.TrustedCAFile.Secret.Name)
			}
		}
	}

	ds, err := dsBuilder.Build()
	if err != nil {
		return ctrl.Result{}, err
	}

	cfgHash := configHash(configmap.Data)
	specHash := podTemplateSpecHash(ds.Spec.Template)
	if ds.Spec.Template.Annotations == nil {
		ds.Spec.Template.Annotations = map[string]string{}
	}
	ds.Spec.Template.Annotations[configHashAnnotation] = cfgHash
	ds.Spec.Template.Annotations[specHashAnnotation] = specHash

	log.Info("set reference daemonset")
	if err := controllerutil.SetControllerReference(client, ds, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("get daemonset")
	createdDS := &appsv1.DaemonSet{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: ds.Name, Namespace: ds.Namespace}, createdDS)
	if err != nil && errors.IsNotFound(err) {
		log.Info("create daemonset")
		if err = r.Client.Create(ctx, ds); err != nil {
			r.setCondition(client, status.ConditionTypeReady, metav1.ConditionFalse, status.ReasonPodFailed, err.Error())
			if statusErr := r.updateClientStatus(ctx, client, status.ClientPhaseFailed, err.Error(), upstreamCount, visitorCount); statusErr != nil {
				log.Error(statusErr, "failed to update client status")
			}
			return ctrl.Result{}, err
		}
		r.Recorder.Event(client, corev1.EventTypeNormal, EventReasonClientConnected,
			fmt.Sprintf("FRP client daemonset created for server %s:%d", client.Spec.Server.Host, client.Spec.Server.Port))
		r.setCondition(client, status.ConditionTypeReady, metav1.ConditionFalse, status.ReasonPodCreated, "DaemonSet created, waiting for pods to start")
		if err := r.updateClientStatus(ctx, client, status.ClientPhasePending, "DaemonSet created, waiting for pods to start", upstreamCount, visitorCount); err != nil {
			log.Error(err, "failed to update client status")
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// Roll the template when either fingerprint annotation diverges.
	// Comparing hashes of the desired (operator-rendered) state avoids the
	// classic update-loop where reflect.DeepEqual against the live PodSpec
	// keeps diverging from server-side defaults (dnsPolicy,
	// terminationGracePeriodSeconds, etc.) and re-Updates every reconcile.
	cfgChanged := createdDS.Spec.Template.Annotations[configHashAnnotation] != cfgHash
	specChanged := createdDS.Spec.Template.Annotations[specHashAnnotation] != specHash
	if cfgChanged || specChanged {
		log.Info("update daemonset template", "cfgChanged", cfgChanged, "specChanged", specChanged)
		createdDS.Spec.Template = ds.Spec.Template
		if err := r.Client.Update(ctx, createdDS); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Readiness: at least one pod available means traffic can flow; full
	// availability is reported via DaemonSet status fields and shown in
	// the Client status message.
	switch {
	case createdDS.Status.NumberAvailable == 0:
		r.setCondition(client, status.ConditionTypeReady, metav1.ConditionFalse, status.ReasonPodCreated, "no DaemonSet pods available yet")
		if err := r.updateClientStatus(ctx, client, status.ClientPhasePending,
			fmt.Sprintf("DaemonSet pods not yet available (desired=%d)", createdDS.Status.DesiredNumberScheduled),
			upstreamCount, visitorCount); err != nil {
			log.Error(err, "failed to update client status")
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	default:
		r.setCondition(client, status.ConditionTypeReady, metav1.ConditionTrue, status.ReasonPodRunning, "FRP client DaemonSet has at least one available pod")
		msg := fmt.Sprintf("Connected to %s:%d (%d/%d daemonset pods available)",
			client.Spec.Server.Host, client.Spec.Server.Port,
			createdDS.Status.NumberAvailable, createdDS.Status.DesiredNumberScheduled)
		if err := r.updateClientStatus(ctx, client, status.ClientPhaseRunning, msg, upstreamCount, visitorCount); err != nil {
			log.Error(err, "failed to update client status")
		}
	}

	// Persist a "no reload pending" state on the ConfigMap so the bare-Pod
	// reconcile semantics (which use this annotation) stay consistent if a
	// Client is flipped between kinds.
	if createdConfigMap.Annotations != nil && createdConfigMap.Annotations[reloadPendingAnnotation] == "true" {
		delete(createdConfigMap.Annotations, reloadPendingAnnotation)
		if err := r.Client.Update(ctx, createdConfigMap); err != nil {
			log.Error(err, "failed to clear reload-pending annotation")
			return ctrl.Result{}, err
		}
	}

	if !reflect.DeepEqual(createdConfigMap.Data, configmap.Data) {
		log.Info("found config diff, update configmap")
		createdConfigMap.Data = configmap.Data
		if err := r.Client.Update(ctx, createdConfigMap); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// podTemplateSpecHash is a stable fingerprint of the operator-rendered pod
// template (excluding the hash annotations themselves). Used to detect drift
// against the live DaemonSet without falsely matching server-side defaults.
func podTemplateSpecHash(tmpl corev1.PodTemplateSpec) string {
	// Strip the hash annotations so the fingerprint covers the rendered
	// content rather than its own checksum.
	if tmpl.Annotations != nil {
		copyAnn := make(map[string]string, len(tmpl.Annotations))
		for k, v := range tmpl.Annotations {
			if k == configHashAnnotation || k == specHashAnnotation {
				continue
			}
			copyAnn[k] = v
		}
		tmpl.Annotations = copyAnn
	}
	b, _ := json.Marshal(tmpl)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:16]
}

// configHash is a stable, short fingerprint of ConfigMap.Data used as the
// pod-template annotation that drives DaemonSet rolling updates.
func configHash(data map[string]string) string {
	h := sha256.New()
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(data[k]))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
