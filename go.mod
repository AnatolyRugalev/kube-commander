module github.com/AnatolyRugalev/kube-commander

go 1.12

require (
	github.com/gdamore/tcell v1.3.0
	github.com/googleapis/gnostic v0.2.0 // indirect
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/kyokomi/emoji v2.1.0+incompatible
	github.com/spf13/cast v1.3.0
	github.com/spf13/cobra v0.0.5
	google.golang.org/appengine v1.6.0 // indirect
	k8s.io/api v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/client-go v0.17.2
	k8s.io/kubectl v0.17.2
)

replace github.com/gdamore/tcell => github.com/AnatolyRugalev/tcell v1.3.1-0.20200302223233-75ad5b357688
