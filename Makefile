DOCKER_HOST := unix:///Users/ris-tlp/.colima/default/docker.sock
TEST_CONTAINER := test-app

.PHONY: run test-up test-down test-restart curl clean

run:
	DOCKER_HOST=$(DOCKER_HOST) go run ./cmd/main.go

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

clean: test-down
