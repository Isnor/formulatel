#!/bin/bash

# TODO: this script routinely fails to execute properly via cloud-init because `apt` fails sometimes,
# but I don't know why
# setup-server.sh is meant to provision a single-node kubernetes cluster and a postgres instance
# on an ubuntu VM.


# allow TCP traffic in for kubernetes, mqtt, and postgres
# accept all traffic on port 6443 - probably a bad idea, restrict to a CIDR. the reason we have this at all
# is so the admin can run kubectl from their workstation instead of logging into the box
iptables -I INPUT 6 -p tcp --dport 6443 -j ACCEPT
iptables -I INPUT -s 10.42.0.0/16 -j ACCEPT # accept all traffic from the local kubernetes cluster
# iptables -I INPUT 6 -p tcp --dport 1883 -j ACCEPT # mqtt - uncomment when we're ready to accept ingest traffic
netfilter-persistent save

# install postgres
sudo apt install -y curl ca-certificates
sudo install -d /usr/share/postgresql-common/pgdg
sudo curl -fsSLo /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc https://www.postgresql.org/media/keys/ACCC4CF8.asc

sudo apt install -y gnupg postgresql-common apt-transport-https lsb-release wget
sudo /usr/share/postgresql-common/pgdg/apt.postgresql.org.sh -v 18

# timescale DB
echo "deb https://packagecloud.io/timescale/timescaledb/ubuntu/ $(lsb_release -c -s) main" | sudo tee /etc/apt/sources.list.d/timescaledb.list
wget --quiet -O - https://packagecloud.io/timescale/timescaledb/gpgkey | sudo gpg --dearmor -o /etc/apt/trusted.gpg.d/timescaledb.gpg
sudo apt update && sudo apt install -y timescaledb-2-postgresql-18 postgresql-client-18

# Configure Postgres to accept connections from the internal K3s network interface
echo "listen_addresses = '*'" >> /etc/postgresql/18/main/postgresql.conf
echo "host	all 		all 		10.42.0.0/16 		scram-sha-256" >> /etc/postgresql/18/main/pg_hba.conf
sudo timescaledb-tune --quiet --yes
sudo systemctl restart postgresql

# TODO: this line is untested; the query is required, but I ran it manually over the CLI after setting up
# the VM the fist time.
sudo -u postgres psql -c "CREATE EXTENSION IF NOT EXISTS timescaledb;"

# install kubernetes
export PUBLIC_IP=$(curl -s ifconfig.me | tr -d '\r\n')
# we need this for the generated certificate to include the public IP of the VM, which we needed to
# run kubectl remotely against the server. In hindsight, I don't know why we should
# need this; why not simply copy the manifests to the server and apply them locally? I suppose this will
# be better if we add more nodes to our cluster, but we need to copy /etc/rancher/k3s/k3s.yaml to our workstation
# and update the `server` part either way, so we're not going to be able to
mkdir -p /etc/rancher/k3s
cat <<EOF |sudo tee /etc/rancher/k3s/config.yaml
write-kubeconfig-mode: "0644"
tls-san:
  - "${PUBLIC_IP}"
cluster-init: true
EOF

# k3s: a small, low-memory distribution of k8s
curl -sfL https://get.k3s.io | sh -s -

kubectl create namespace formulatel