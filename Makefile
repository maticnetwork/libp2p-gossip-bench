
deploy-network:
	./scripts/e2e-deploy-network.sh $(nodes) $(maxPeers)

clean:
	./scripts/e2e-clean.sh

build:
	docker build -t yyyy .
