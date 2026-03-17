#!/bin/bash

kind create cluster --image=kindest/node:v1.35.0

helm upgrade --install \
  cert-manager oci://quay.io/jetstack/charts/cert-manager \
  --version v1.19.1 \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true

helm install eg oci://docker.io/envoyproxy/gateway-helm --version v1.7.1 -n envoy-gateway-system --create-namespace --wait
kubectl apply -f https://github.com/envoyproxy/gateway/releases/download/v1.7.1/quickstart.yaml -n default

