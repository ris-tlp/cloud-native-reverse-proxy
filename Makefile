DOCKER_HOST := unix:///Users/ris-tlp/.colima/default/docker.sock
COLIMA_SOCK := /Users/ris-tlp/.colima/default/docker.sock
TEST_CONTAINER := test-app
PROXY_CONTAINER := proxy
PROXY_IMAGE := proxy:dev

.PHONY: run build-image run-container stop-container test-up test-down test-restart curl clean

# local dev — runs on host (won't reach container IPs on Colima/Docker Desktop)
run:
	DOCKER_HOST=$(DOCKER_HOST) go run ./cmd/main.go

# build the proxy as a Docker image
build-image:
	docker build -t $(PROXY_IMAGE) .

# run the proxy in Docker — same network as test containers, IPs reachable
run-container: stop-container
	docker run -d \
		--name $(PROXY_CONTAINER) \
		-p 8080:8080 \
		-v /var/run/docker.sock:/var/run/docker.sock \
		$(PROXY_IMAGE)
	docker logs -f $(PROXY_CONTAINER)

stop-container:
	-docker stop $(PROXY_CONTAINER)
	-docker rm $(PROXY_CONTAINER)

test-up:
	docker run -d \
		--name $(TEST_CONTAINER) \
		--label proxy.host=test.localhost \
		--label proxy.port=80 \
		nginx

test-down:
	-docker stop $(TEST_CONTAINER)
	-docker rm $(TEST_CONTAINER)

test-restart:
	docker restart $(TEST_CONTAINER)

curl:
	curl -H "Host: test.localhost" http://localhost:8080

clean: test-down stop-container
