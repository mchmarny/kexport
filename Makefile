PROJECT=$(gcloud config get-value project)

.PHONY: clean

mod:
	go mod tidy
	go mod vendor

image: mod
	gcloud builds submit \
		--project cloudylabs-public \
		--tag gcr.io/cloudylabs-public/kexport:0.3.4

pod:
	kubectl run kexport --env="INTERVAL=30s" \
		--replicas=1 --generator=run-pod/v1 \
		--image=gcr.io/cloudylabs-public/kexport:0.3.4

podless:
	kubectl delete pod kexport
