docker_host := env_var_or_default("DOCKER_HOST", "unix:///Users/ris-tlp/.colima/default/docker.sock") # colima shenanigans
proxy_container := "proxy"
proxy_image := "proxy:dev"

# build the proxy image and load it into minikube
[private]
[group('K8s provider :: proxy')]
k8s-proxy-build:
    docker build -t {{proxy_image}} .
    minikube image load {{proxy_image}}

# deploy the proxy and wait for the rollout
[private]
[group('K8s provider :: proxy')]
k8s-proxy-up:
    kubectl apply -k manifests/dev/cnrp
    kubectl rollout status deployment/cnrp-deployment --timeout=60s

# rebuild, redeploy, and restart the proxy
[group('K8s provider :: proxy')]
k8s-proxy-reload: k8s-proxy-build k8s-proxy-up
    kubectl rollout restart deployment/cnrp-deployment
    kubectl rollout status deployment/cnrp-deployment --timeout=60s

# delete the proxy from the cluster
[private]
[group('K8s provider :: proxy')]
k8s-proxy-down:
    -kubectl delete -k manifests/dev/cnrp

# tail logs of the newest proxy pod
[private]
[group('K8s provider :: proxy')]
k8s-proxy-logs:
    kubectl logs -f $(kubectl get pod -l app=cnrp --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[-1:].metadata.name}')

# port-forward the proxy Service to localhost:8080
[group('K8s provider :: proxy')]
k8s-proxy-forward:
    kubectl port-forward service/cnrp-svc 8080:8080

# full dev loop: bring up test apps, reload proxy, then tail logs
[group('K8s provider :: proxy')]
k8s-dev-up: k8s-test-up k8s-proxy-reload
    just k8s-proxy-logs

# deploy the test app 
[group('K8s provider :: backends')]
k8s-test-up:
    kubectl apply -k manifests/dev/test-app

# remove the test app
[group('K8s provider :: backends')]
k8s-test-down:
    -kubectl delete -k manifests/dev/test-app

# curl the test app through the proxy (needs port-forward)
[group('K8s provider :: backends')]
k8s-test-curl:
    curl -H "Host: test.localhost" http://localhost:8080

# build the proxy docker image
[private]
[group('Docker provider :: proxy')]
docker-build-image:
    docker build -t {{proxy_image}} .

# full dev loop: bring up 3 test backends, then build/run the proxy and tail logs
[group('Docker provider :: proxy')]
docker-dev-up: docker-test-up docker-run

# build the image and run the proxy container
[private]
[group('Docker provider :: proxy')]
docker-run: docker-build-image docker-run-container

# run the proxy container and tail its logs
[private]
[group('Docker provider :: proxy')]
docker-run-container: docker-stop-container
    docker run -d \
        --name {{proxy_container}} \
        -p 8080:8080 \
        -v /var/run/docker.sock:/var/run/docker.sock \
        -v $(pwd)/cnrp.toml:/cnrp.toml \
        {{proxy_image}} -config /cnrp.toml
    docker logs -f {{proxy_container}}

# stop and remove the proxy container
[private]
[group('Docker provider :: proxy')]
docker-stop-container:
    -docker stop {{proxy_container}}
    -docker rm {{proxy_container}}

# run N http-echo backends on test.localhost (default 3, for load-balancing tests)
[group('Docker provider :: backends')]
docker-test-up n="3":
    for i in $(seq 1 {{n}}); do \
        docker run -d --name backend-$i \
            --label proxy.host=test.localhost \
            --label proxy.port=5678 \
            hashicorp/http-echo -text="backend-$i"; \
    done

# remove all backend-* containers
[group('Docker provider :: backends')]
docker-test-down:
    -docker rm -f $(docker ps -aq --filter "name=^backend-")

# restart a single backend by index, e.g. just docker-test-restart 2
[group('Docker provider :: backends')]
docker-test-restart n:
    docker restart backend-{{n}}

# remove a single backend by index, e.g. just docker-test-down-one 2
[group('Docker provider :: backends')]
docker-test-down-one n:
    docker stop backend-{{n}} && docker rm backend-{{n}}

# curl the proxy on test.localhost
[group('Docker provider :: backends')]
docker-test-curl:
    curl -H "Host: test.localhost" http://localhost:8080

# run unit tests
[group('Tests')]
test:
    go test ./...

# run integration tests against docker
[group('Tests')]
test-integration:
    DOCKER_HOST={{docker_host}} go test ./integration/ -tags=integration -v

# tear down all docker + k8s test resources
[group('Cleanup')]
clean: docker-test-down docker-stop-container k8s-proxy-down k8s-test-down
