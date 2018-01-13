/*
Copyright 2017 The Kubernetes Authors.

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

package recommender

import (
	"crypto/sha1"
	"fmt"
	"runtime"
	"time"

	"k8s.io/autoscaler/vertical-pod-autoscaler/apimock"

	apiv1 "k8s.io/api/core/v1"
	hashutil "k8s.io/kubernetes/pkg/util/hash"
)

// CachingRecommender provides VPA recommendations for pods.
// VPA responses are cached.
type CachingRecommender interface {
	// Get returns VPA recommendation for given pod
	Get(spec *apiv1.PodSpec) (*apimock.Recommendation, error)
}

type cachingRecommenderImpl struct {
	api   apimock.RecommenderAPI
	cache *TTLCache
}

// NewCachingRecommender creates CachingRecommender with given cache TTL
func NewCachingRecommender(ttl time.Duration, api apimock.RecommenderAPI) CachingRecommender {
	ca := NewTTLCache(ttl)
	ca.StartCacheGC(ttl)

	result := &cachingRecommenderImpl{api: api, cache: ca}
	// We need to stop background cacheGC worker if cachingRecommenderImpl gets destryed.
	// If we forget this, background go routine will forever run and hold a reference to TTLCache object.
	runtime.SetFinalizer(result, stopChacheGC)

	return result
}

// Get returns VPA recommendation for the given pod. If recommendation is not in cache, sends request to RecommenderAPI
func (c *cachingRecommenderImpl) Get(spec *apiv1.PodSpec) (*apimock.Recommendation, error) {
	cacheKey := getCacheKey(spec)
	if cacheKey != nil {
		if cached := c.cache.Get(cacheKey); cached != nil {
			return cached.(*apimock.Recommendation), nil
		}
	}

	response, err := c.api.GetRecommendation(spec)
	if err != nil {
		return nil, fmt.Errorf("error fetching recommendation %v", err)
	}
	if response != nil && cacheKey != nil {
		c.cache.Set(cacheKey, response)
	}
	return response, nil
}

func getCacheKey(spec *apiv1.PodSpec) *string {
	podTemplateSpecHasher := sha1.New()
	hashutil.DeepHashObject(podTemplateSpecHasher, *spec)
	result := string(podTemplateSpecHasher.Sum(make([]byte, 0)))
	return &result
}

func stopChacheGC(c *cachingRecommenderImpl) {
	c.cache.StopCacheGC()
}
