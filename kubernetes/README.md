# kubernetes node setup

This document is very rough and contains some of the manual setup required for the shared postgres/kubernetes node that I wasn't able to script. It's basically creating a password for the `postgres` role and adding a secret to kubernetes

## download the kubeconfig

After the node has been provisioned and you can access it via SSH, use `scp` to download the `kubeconfig` for the server and update the context name, server-name, and user:

```bash
ssh formulatel.oci "sudo cat /etc/rancher/k3s/k3s.yaml" > ~/.kube/formulatel-staging-config
```

You can update your `KUBECONFIG` envar to include the path to the new file. Run `kubectl config get-contexts` to show your available contexts.

## set the `postgres` user password

After postgres is installed on the postgres/k8s VM, run `sudo -u postgres psql` to launch login to the DB and `\password` to set the password for the `postgres` user. You will need to create a secret in Kubernetes for the db-migrate and persist pods to be able to login to the DB.

Eventually, create roles for db-migrate and persist instead of using the `postgres` superuser.

## create DB secrets

After you have user credentials to create secrets from, use `kubectl` to add them to kubernetes:

```bash
kubectl create secret generic formulatel-db-secrets --from-literal='username=formulatel_persist' --from-literal='password=12345' --from-literal='host=10.0.1.38' --from-literal='port=5432' --namespace=formulatel
```

**Note**: The `10.0.1.38` comes from k3s; it is the IP of the default CNI that gets setup. If your node isn't using k3s, that IP might not make sense.

## install formulatel

Use the included chart to install `mqtt`, `grafana`, `persist` and `db-migrate`

```bash
helm dependency build kubernetes/charts/formulatel
helm --kube-context formulatel-staging --namespace formulatel install formulatel ./kubernetes/charts/formulatel --values=./kubernetes/charts/formulatel/values.yaml
```

To update the running service, use `helm upgrade`

## configure Grafana datasources

Since we can't exactly store passwords in plaintext, the Grafana values don't have the datasources like postgres setup automatically like they do when we `tilt up` locally. Log in to Grafana and setup the data sources manually.

### (optional) forward ports

For local testing before setting up any DNS records, you will have to forward the Grafana and mosquitto ports:

```bash
kubectl --namespace formulatel port-forward $POD_NAME 1883|3000
```

## start ingest

Make sure to set the MQTT_BROKER envar before running ingest to point to the remote MQTT broker

**With DNS**:
```bash
export MQTT_BROKER='tcp://mqtt.formula.tel:1883'
```

**With Port-Forwarding**:

```bash
export MQTT_BROKER='tcp://127.0.0.1:1883'
```