// Package subjectaccess provides functions for listing resource access in a Kubernetes cluster.
package subjectaccess

import (
	"context"
	"fmt"
	"log"
	"sync"

	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/discovery"
	authClient "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

const (
	Denied int = iota
	Allowed
	Unused
	Error
)

var (
	APIVerbs = []string{
		"create",
		"get",
		"list",
		"watch",
		"update",
		"patch",
		"delete",
		"deletecollection",
	}
)

// Resource holds an APIGroup and APIResource pair. This is used as a
// key by ResourceAccess mapping Resource to verbs.
type Resource struct {
	APIGroup    string
	APIResource metav1.APIResource
}

// String provides a serializable string representation of a Resource
func (r Resource) String() string {
	if r.APIGroup == "" {
		return r.APIResource.Name
	}
	return fmt.Sprintf("%s.%s", r.APIGroup, r.APIResource.Name)
}

// ResourceList creates a list of Resource objects using the Discovery client.
func ResourceList(_ context.Context, client discovery.DiscoveryInterface, namespace string) ([]Resource, error) {
	var resourceList func() ([]*metav1.APIResourceList, error)
	if namespace == "" {
		resourceList = client.ServerPreferredResources
	} else {
		resourceList = client.ServerPreferredNamespacedResources
	}

	resources, err := resourceList()
	if err != nil {
		if resources == nil {
			return nil, fmt.Errorf("get preferred resources: %w", err)
		}
		log.Printf("Unable to get full resource list: %s", err)
	}

	var result []Resource
	for _, resp := range resources {
		if len(resp.APIResources) == 0 {
			continue
		}

		groupVersion, err := schema.ParseGroupVersion(resp.GroupVersion)
		if err != nil {
			log.Printf("Unable to parse groupVersion: %s", err)
			continue
		}

		for _, r := range resp.APIResources {
			if len(r.Verbs) == 0 {
				continue
			}

			result = append(result, Resource{
				APIGroup:    groupVersion.Group,
				APIResource: r,
			})
		}
	}

	return result, nil
}

func resourceVerbKey(resource Resource, verb string) string {
	return fmt.Sprintf("%s.%s", resource, verb)
}

// ResourceAccess provides a way to check if a given resource and verb are allowed to be performed by
// the current Kubernetes client.
type ResourceAccess interface {
	Allowed(resource Resource, verb string)
	String() string
}

// NewResourceAccess provides a ResourceAccess object with an access map popluated from issuing SelfSubjectAccessReview
// requests for the list of resources and verbs provided.
func NewResourceAccess(ctx context.Context, client authClient.SelfSubjectAccessReviewInterface, namespace string, resources []Resource) *resourceAccess {
	ra := &resourceAccess{
		access: sync.Map{},
	}

	group := sync.WaitGroup{}
	for _, resource := range resources {
		group.Add(1)

		resource := resource
		namespace := namespace

		go func(ctx context.Context) {
			select {
			case <-ctx.Done():
				return
			default:
			}

			if !resource.APIResource.Namespaced {
				namespace = ""
			}
			apiVerbs := sets.NewString(resource.APIResource.Verbs...)

			for _, verb := range APIVerbs {
				select {
				case <-ctx.Done():
					return
				default:
				}

				key := resourceVerbKey(resource, verb)

				if !apiVerbs.Has(verb) {
					ra.access.Store(key, Unused)
					continue
				}

				sar := &authv1.SelfSubjectAccessReview{
					Spec: authv1.SelfSubjectAccessReviewSpec{
						ResourceAttributes: &authv1.ResourceAttributes{
							Verb:      verb,
							Resource:  resource.APIResource.Name,
							Group:     resource.APIGroup,
							Namespace: namespace,
						},
					},
				}

				if result, err := client.Create(ctx, sar, metav1.CreateOptions{}); err != nil {
					ra.access.Store(key, Error)
				} else {
					if result.Status.Allowed {
						ra.access.Store(key, Allowed)
					} else {
						ra.access.Store(key, Denied)
					}
				}
			}

			group.Done()
		}(ctx)
	}

	group.Wait()

	return ra
}

type resourceAccess struct {
	access sync.Map
}

func (r *resourceAccess) Allowed(resource Resource, verb string) bool {
	key := resourceVerbKey(resource, verb)

	v, found := r.access.Load(key)
	if !found {
		log.Printf("verb %s not found for %s", verb, resource)
		return false
	}

	s, ok := v.(int)
	if !ok {
		log.Printf("unable to type convert status to int, malformed access map")
		return false
	}

	return statusIntAsBool(s)
}

func (r *resourceAccess) String() string {
	result := ""
	printer := func(key, value interface{}) bool {
		s, ok := key.(string)
		if !ok {
			return false
		}

		v, ok := value.(int)
		if !ok {
			return false
		}

		result += fmt.Sprintf("%s: %d\n", s, v)

		return true
	}
	r.access.Range(printer)
	return result
}

func statusIntAsBool(i int) bool {
	if i == Denied || i == Unused {
		return false
	}
	if i == Allowed {
		return true
	}
	return false
}
