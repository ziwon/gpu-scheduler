SCHED_IMG ?= ghcr.io/ziwon/gpu-scheduler:dev
WEBHOOK_IMG ?= ghcr.io/ziwon/gpu-scheduler-webhook:dev
AGENT_IMG ?= ghcr.io/ziwon/gpu-scheduler-agent:dev

.PHONY: docker docker-webhook docker-agent
docker:
	docker build --build-arg CMD_PATH=cmd/scheduler -t $(SCHED_IMG) .

docker-webhook:
	docker build --build-arg CMD_PATH=cmd/webhook -t $(WEBHOOK_IMG) .

docker-agent:
	docker build --build-arg CMD_PATH=cmd/agent -t $(AGENT_IMG) .
