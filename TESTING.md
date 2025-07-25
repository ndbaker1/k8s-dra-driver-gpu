```bash
export REGISTRY=
export VERSION=dev

make -f deployments/container/Makefile build
docker push $REGISTRY/k8s-dra-driver-gpu:$VERSION

helm upgrade -i --create-namespace --namespace nvidia nvidia-dra-driver-gpu deployments/helm/nvidia-dra-driver-gpu \
   --set image.repository=$REGISTRY/k8s-dra-driver-gpu \
   --set image.tag=$VERSION \
   --set image.pullPolicy=Always \
   --set resources.gpus.enabled=false

kubectl rollout restart daemonset -n nvidia nvidia-dra-driver-gpu-kubelet-plugin
kubectl rollout restart deployment -n nvidia nvidia-dra-driver-gpu-controller
```
