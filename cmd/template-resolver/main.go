package main

import (
  "context"
  "errors"
  
  "github.com/tektoncd/pipeline/pkg/resolution/common"
  "github.com/tektoncd/pipeline/pkg/remoteresolution/resolver/framework"
  "knative.dev/pkg/injection/sharedmain"
  pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

func main() {
  sharedmain.Main("controller",
    framework.NewController(context.Background(), &resolver{}),
  )
}

type resolver struct {}
// Initialize sets up any dependencies needed by the resolver. None atm.
func (r *resolver) Initialize(context.Context) error {
  return nil
}

// GetName returns a string name to refer to this resolver by.
func (r *resolver) GetName(context.Context) string {
  return "Demo"
}

// GetSelector returns a map of labels to match requests to this resolver.
func (r *resolver) GetSelector(context.Context) map[string]string {
  return map[string]string{
    common.LabelKeyResolverType: "demo",
  }
}

// GetSelector returns a map of labels to match requests to this resolver.
func (r *resolver) GetSelector(context.Context) map[string]string {
  return map[string]string{
    common.LabelKeyResolverType: "demo",
  }
}

// Validate ensures that the resolution spec from a request is as expected.
func (r *resolver) Validate(ctx context.Context, req *v1beta1.ResolutionRequestSpec) error {
  if len(req.Params) > 0 {
    return errors.New("no params allowed")
  }
  url := req.URL
  u, err := neturl.ParseRequestURI(url)
  if err != nil {
    return err
  }
  if u.Scheme != "demoscheme" {
    return fmt.Errorf("Invalid Scheme. Want %s, Got %s", "demoscheme", u.Scheme)
  }
  if u.Path == "" {
    return errors.New("Empty path.")
  }
  return nil
}

// Resolve uses the given resolution spec to resolve the requested file or resource.
func (r *resolver) Resolve(ctx context.Context, req *v1beta1.ResolutionRequestSpec) (framework.ResolvedResource, error) {
  return &myResolvedResource{}, nil
}

// our hard-coded resolved file to return
const pipeline = `
apiVersion: tekton.dev/v1beta1
kind: Pipeline
metadata:
  name: my-pipeline
spec:
  tasks:
  - name: hello-world
    taskSpec:
      steps:
      - image: alpine:3.15.1
        script: |
          echo "hello world"
`

// myResolvedResource wraps the data we want to return to Pipelines
type myResolvedResource struct {}

// Data returns the bytes of our hard-coded Pipeline
func (*myResolvedResource) Data() []byte {
  return []byte(pipeline)
}

// Annotations returns any metadata needed alongside the data. None atm.
func (*myResolvedResource) Annotations() map[string]string {
  return nil
}

// RefSource is the source reference of the remote data that records where the remote 
// file came from including the url, digest and the entrypoint. None atm.
func (*myResolvedResource) RefSource() *pipelinev1.RefSource {
	return nil
}


