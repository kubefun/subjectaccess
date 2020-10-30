/*
Copyright 2016 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/wwitzel3/subjectaccess/pkg/subjectaccess"
)

func main() {
	var kubeconfig *string
	if home := homeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	config.QPS = 500
	config.Burst = 1000

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	startDefault := time.Now()
	resources, err := subjectaccess.ResourceList(context.TODO(), clientset.Discovery(), "default")
	if err != nil {
		panic(err.Error())
	}
	elapsedDefault := time.Since(startDefault)
	fmt.Printf("Took (ns: default)): %s\n", elapsedDefault)

	startCluster := time.Now()
	_, err = subjectaccess.ResourceList(context.TODO(), clientset.Discovery(), "")
	if err != nil {
		panic(err.Error())
	}
	elapsedCluster := time.Since(startCluster)
	fmt.Printf("Took (cluster)): %s\n", elapsedCluster)

	// Namespace default
	resourceAccess := subjectaccess.NewResourceAccess(context.TODO(), clientset.AuthorizationV1().SelfSubjectAccessReviews(), resources)
	pod := subjectaccess.Resource{
		GroupVersionKind: schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "Pod",
		},
		Namespace: "default",
	}

	fmt.Println("Can get v1/Pod?", resourceAccess.Allowed(pod, "get"))
	fmt.Println("Can get/list/watch v1/Pod?", resourceAccess.AllowedAll(pod, []string{"get", "list", "watch"}))

	resources, err = subjectaccess.ResourceList(context.TODO(), clientset.Discovery(), "")
	if err != nil {
		panic(err.Error())
	}

	// No namespace
	resourceAccess = subjectaccess.NewResourceAccess(context.TODO(), clientset.AuthorizationV1().SelfSubjectAccessReviews(), resources)
	ns := subjectaccess.Resource{
		GroupVersionKind: schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "Namespace",
		},
	}

	fmt.Println("Can delete v1/Namespace?", resourceAccess.Allowed(ns, "delete"))
	fmt.Println("Can deletecollection v1/Namespace?", resourceAccess.Allowed(ns, "deletecollection"))

	pod.Namespace = ""
	fmt.Println("Can get v1/Pod?", resourceAccess.Allowed(pod, "get"))
	fmt.Println("Can get/list/watch v1/Pod?", resourceAccess.AllowedAll(pod, []string{"get", "list", "watch"}))
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
