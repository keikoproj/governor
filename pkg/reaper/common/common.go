/*

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

package common

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Log is the common logger
var Log = logrus.New()

// HomeDir gets the current user's homedir
func HomeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return ""
}

// PathExists checks whether a given path exists or not
func PathExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}

// InClusterAuth returns an in-cluster kubernetes client
func InClusterAuth() (*kubernetes.Clientset, error) {
	Log.Infoln("starting in-cluster auth")

	config, err := rest.InClusterConfig()
	if err != nil {
		return &kubernetes.Clientset{}, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return &kubernetes.Clientset{}, err
	}

	return clientset, nil
}

// OutOfClusterAuth returns an external kubernetes client
func OutOfClusterAuth(providedConfigPath string) (*kubernetes.Clientset, error) {
	Log.Infoln("starting cluster external auth")

	var configPath string

	if providedConfigPath != "" {
		configPath = providedConfigPath
	} else if HomeDir() != "" {
		configPath = filepath.Join(HomeDir(), ".kube", "config")
	} else {
		err := fmt.Sprintf("could not find valid kubeconfig file")
		Log.Errorln(err)
		return &kubernetes.Clientset{}, fmt.Errorf("%s", err)
	}

	Log.Infof("kubeconfig: %v\n", configPath)

	config, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		return &kubernetes.Clientset{}, err
	}

	Log.Infof("target: %v\n", config.Host)

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return &kubernetes.Clientset{}, err
	}

	return clientset, nil
}

func GetSelectorString(selector *metav1.LabelSelector) (string, error) {
	selectorMap, err := metav1.LabelSelectorAsMap(selector)
	if err != nil {
		return "", errors.Wrapf(err, "failed to convert PDB selector to map %+v", selector)
	}

	return labels.SelectorFromSet(selectorMap).String(), nil
}

func StringSliceContains(sl []string, s string) bool {
	for _, x := range sl {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}
