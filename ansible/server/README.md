# RFP Server Ansible

This simple ansible to setup FRP server on the server that has public IP address and prerequisite of frp-operator on Kubernetes.

### Setup Guide

1. SSH to the VM and get admin access
```shell
sudo su
```
2. clone the repository
```shell
git clone https://github.com/zufardhiyaulhaq/frp-operator
cd frp-operator/ansible/server
```
3. Adjust variables
```shell
vi group_vars/all.yaml
```
4. Install ansible
```shell
sudo apt-add-repository ppa:ansible/ansible -y
sudo apt update
sudo apt install ansible -y
```
5. disable ansible hostkey checking
```shell
vi ~/.ansible.cfg

[defaults]
host_key_checking = False
```
6. Run ansible
```
ansible-playbook main.yml -i hosts/hosts
```


