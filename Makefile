RELEASE=0.2.1
PROJECT=$(gcloud config get-value project)

.PHONY: clean

mod:
	go mod tidy
	go mod vendor

image: mod
	gcloud builds submit \
		--project $(PROJECT)-public \
		--tag gcr.io/$(PROJECT)-public/kexport:$(RELEASE)

run:
	kubectl run kexport --replicas=1 --generator=run-pod/v1 \
		--image=gcr.io/$(PROJECT)-public/kexport:$(RELEASE)
