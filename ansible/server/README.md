# FRP Server Ansible

This simple ansible to setup FRP server on the server that has public IP address and prerequisite of frp-operator on Kubernetes.

### Setup Guide

1. SSH to the VM and get admin access
```shell
sudo su
```
2. generate ssh key and make sure to able to ssh to itself
```shell
ssh-keygen

vi ~/.ssh/authorized_keys
```
3. clone the repository
```shell
git clone https://github.com/zufardhiyaulhaq/frp-operator
cd frp-operator/ansible/server
```
1. Adjust variables
```shell
vi group_vars/all.yml
```
1. Install ansible
```shell
sudo apt-add-repository ppa:ansible/ansible -y
sudo apt update
sudo apt install ansible -y
```
1. disable ansible hostkey checking
```shell
vi ~/.ansible.cfg

[defaults]
host_key_checking = False
```
7. Run ansible
```
ansible-playbook main.yml -i hosts/hosts
```


