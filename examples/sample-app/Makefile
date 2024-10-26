build-delth-image:
	docker build -t localhost:5000/delth/sample-app-print-env/with-delth:latest -f Dockerfile ../..

build-non-delth-image:
	docker build -t localhost:5000/delth/sample-app-print-env/without-delth:latest -f Dockerfile.without_delth .

push-delth-image:
	docker push localhost:5000/delth/sample-app-print-env/with-delth:latest

push-non-delth-image:
	docker push localhost:5000/delth/sample-app-print-env/without-delth:latest

build-and-push: build-delth-image build-non-delth-image push-delth-image push-non-delth-image
