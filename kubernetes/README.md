# kubernetes node setup

This document is very rough and contains some of the manual setup required for the shared postgres/kubernetes node that I wasn't able to script. It's basically creating a password for the `postgres` role and adding a secret to kubernetes

## set the `postgres` user password

After postgres is installed on the postgres/k8s VM, run `sudo -u postgres psql` to launch login to the DB and `\password` to set the password for the `postgres` user. You will need to create a secret in Kubernetes for the db-migrate and persist pods to be able to login to the DB.

Eventually, create roles for db-migrate and persist instead of using the `postgres` superuser.

## create DB secrets

After you have user credentials to create secrets from, use `kubectl` to add them to kubernetes:

```bash
kubectl create secret generic formulatel-db-secrets --from-literal='username=formulatel_persist' --from-literal='password=12345' --from-literal='host=10.42.0.1' --from-literal='port=5432' --namespace=formulatel
```

**Note**: The `10.42.0.1` comes from k3s; it is the IP of the default CNI that gets setup. If your node isn't using k3s, that IP might not make sense.