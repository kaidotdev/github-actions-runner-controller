module github-actions-runner-controller

go 1.13

require (
	github.com/go-logr/logr v0.1.0
	github.com/google/goexpect v0.0.0-20191001010744-5b6988669ffa
	github.com/google/goterm v0.0.0-20190703233501-fc88cf888a3f // indirect
	google.golang.org/grpc v1.27.1 // indirect
	k8s.io/api v0.17.9
	k8s.io/apimachinery v0.17.9
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	sigs.k8s.io/controller-runtime v0.5.2
)

replace k8s.io/client-go => k8s.io/client-go v0.17.9
