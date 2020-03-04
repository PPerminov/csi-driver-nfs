#!/usr/bin/env bash

set -ex
kubectx minikube
kubectl delete -f examples/kubernetes/folder_creation.yaml || echo
kubectl delete -f deploy/kubernetes || echo
make build
docker build . -t  pavelatcai/nfs:latest
docker push pavelatcai/nfs:latest
kubectx minikube
kubectl apply -f deploy/kubernetes
kubectl apply -f examples/kubernetes/folder_creation.yaml