docker_host := "unix:///Users/ris-tlp/.colima/default/docker.sock"
test_container := "test-app"
proxy_container := "proxy"
proxy_image := "proxy:dev"

[group('automated tests')]
test:
    go test ./...

[group('automated tests')]
test-integration:
    DOCKER_HOST={{docker_host}} go test ./integration/ -tags=integration -v

[group('manual tests')]
test-up:
    docker run -d \
        --name {{test_container}} \
        --label proxy.host=test.localhost \
        --label proxy.port=80 \
        nginx

[group('manual tests')]
test-down:
    -docker stop {{test_container}}
    -docker rm {{test_container}}

[group('manual tests')]
test-restart:
    docker restart {{test_container}}

[group('manual tests')]
test-curl:
    curl -H "Host: test.localhost" http://localhost:8080

[group('manual tests')]
test-up-multi n="3":
    for i in $(seq 1 {{n}}); do \
        docker run -d --name backend-$i \
            --label proxy.host=test.localhost \
            --label proxy.port=5678 \
            hashicorp/http-echo -text="backend-$i"; \
    done

[group('manual tests')]
test-down-multi n="3":
    -for i in $(seq 1 {{n}}); do \
        docker stop backend-$i && docker rm backend-$i; \
    done

[group('manual tests')]
test-down-one n:
    docker stop backend-{{n}} && docker rm backend-{{n}}

[group('manual tests')]
curl-multi:
    for i in $(seq 1 9); do curl -s -H "Host: test.localhost" http://localhost:8080; done

[group('containers')]
run: build-image run-container

[group('containers')]
build-image:
    docker build -t {{proxy_image}} .

[group('containers')]
run-container: stop-container
    docker run -d \
        --name {{proxy_container}} \
        -p 8080:8080 \
        -v /var/run/docker.sock:/var/run/docker.sock \
        {{proxy_image}}
    docker logs -f {{proxy_container}}

[group('containers')]
stop-container:
    -docker stop {{proxy_container}}
    -docker rm {{proxy_container}}

[group('containers')]
clean n="3": test-down stop-container
    -for i in $(seq 1 {{n}}); do \
        docker stop backend-$i && docker rm backend-$i; \
    done
