#!/bin/bash

echo "Creating certificates"
mkdir certs
openssl req -nodes -new -x509 -keyout certs/ca.key -out certs/ca.crt -subj "/CN=Admission Controller Demo"
openssl genrsa -out certs/admission-tls.key 2048
openssl req -new -key certs/admission-tls.key -subj "/CN=mutate-server-svc.mutate-webhook.svc" | openssl x509 -req -CA certs/ca.crt -CAkey certs/ca.key -CAcreateserial -out certs/admission-tls.crt

echo "Creating k8s Secret"
kubectl create -n mutate-webhook secret tls admission-tls \
    --cert "certs/admission-tls.crt" \
    --key "certs/admission-tls.key"

echo "Creating k8s admission deployment"
kubectl create -f deploy/mutate-deployment.yaml

echo "Creating k8s webhooks for demo"
CA_BUNDLE=$(cat certs/ca.crt | base64 | tr -d '\n')
cat deploy/webhook-config.yaml | sed "s|\${CA_BUNDLE}|${CA_BUNDLE}|g" | kubectl apply -n mutate-webhook -f -