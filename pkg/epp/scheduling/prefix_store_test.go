/*
Copyright 2025 The Kubernetes Authors.

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

package scheduling

import (
	"context"
	"testing"
	"time"

	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// TestBasicPrefixOperations tests the basic functionality of adding and finding prefixes
func TestBasicPrefixOperations(t *testing.T) {
	ctx := context.Background()
	logger := log.FromContext(ctx)
	ctx = log.IntoContext(ctx, logger)

	config := PrefixStoreConfig{
		MaxEntries:   100,
		MinPrefixLen: 3,
		MaxPrefixLen: 10,
		EntryTTL:     1 * time.Hour,
	}

	store := NewPrefixStore(config)
	podName := k8stypes.NamespacedName{
		Name:      "pod1",
		Namespace: "default",
	}

	// Test adding a prefix
	err := store.AddPrefix(ctx, "hello", podName, "model1")
	if err != nil {
		t.Errorf("Failed to add prefix: %v", err)
	}

	// Test finding the exact prefix
	foundPod, found := store.FindPodForPrefix(ctx, "hello", "model1")
	if !found {
		t.Error("Expected to find prefix")
	}
	if foundPod != podName {
		t.Errorf("Expected pod %v, got %v", podName, foundPod)
	}

	// Test finding with a longer prefix
	foundPod, found = store.FindPodForPrefix(ctx, "hello world", "model1")
	if !found {
		t.Error("Expected to find prefix with longer input")
	}
	if foundPod != podName {
		t.Errorf("Expected pod %v, got %v", podName, foundPod)
	}

	// Test updating an existing prefix
	err = store.AddPrefix(ctx, "hello", podName, "model1")
	if err != nil {
		t.Errorf("Failed to update prefix: %v", err)
	}
}

// TestPrefixLengthConstraints tests the handling of prefixes that are too short or too long
func TestPrefixLengthConstraints(t *testing.T) {
	ctx := context.Background()
	logger := log.FromContext(ctx)
	ctx = log.IntoContext(ctx, logger)

	config := PrefixStoreConfig{
		MaxEntries:   100,
		MinPrefixLen: 3,
		MaxPrefixLen: 10,
		EntryTTL:     1 * time.Hour,
	}

	store := NewPrefixStore(config)
	podName := k8stypes.NamespacedName{
		Name:      "pod1",
		Namespace: "default",
	}

	// Test adding a prefix that's too short
	err := store.AddPrefix(ctx, "hi", podName, "model1")
	if err == nil {
		t.Error("Expected error for prefix that's too short")
	}

	// Test adding a prefix that's too long (should be truncated)
	longPrefix := "this is a very long prefix"
	err = store.AddPrefix(ctx, longPrefix, podName, "model1")
	if err != nil {
		t.Errorf("Expected success when adding long prefix (should be truncated): %v", err)
	}

	// Test finding with the truncated version
	truncatedPrefix := longPrefix[:10] // MaxPrefixLen is 10
	foundPod, found := store.FindPodForPrefix(ctx, truncatedPrefix, "model1")
	if !found {
		t.Error("Expected to find truncated prefix")
	}
	if foundPod != podName {
		t.Errorf("Expected pod %v, got %v", podName, foundPod)
	}

	// Test finding with the full long prefix (should match the truncated version)
	foundPod, found = store.FindPodForPrefix(ctx, longPrefix, "model1")
	if !found {
		t.Error("Expected to find pod with full long prefix (should match truncated version)")
	}
	if foundPod != podName {
		t.Errorf("Expected pod %v, got %v", podName, foundPod)
	}

	// Test finding with a prefix that's too short
	_, found = store.FindPodForPrefix(ctx, "hi", "model1")
	if found {
		t.Error("Expected not to find prefix that's too short")
	}
}

// TestModelNameMatching tests that prefixes are only matched when the model name matches
func TestModelNameMatching(t *testing.T) {
	ctx := context.Background()
	logger := log.FromContext(ctx)
	ctx = log.IntoContext(ctx, logger)

	config := PrefixStoreConfig{
		MaxEntries:   100,
		MinPrefixLen: 3,
		MaxPrefixLen: 10,
		EntryTTL:     1 * time.Hour,
	}

	store := NewPrefixStore(config)
	podName := k8stypes.NamespacedName{
		Name:      "pod1",
		Namespace: "default",
	}

	// Add prefix with model1
	err := store.AddPrefix(ctx, "hello", podName, "model1")
	if err != nil {
		t.Errorf("Failed to add prefix: %v", err)
	}

	// Test finding with same model name
	_, found := store.FindPodForPrefix(ctx, "hello", "model1")
	if !found {
		t.Error("Expected to find prefix with matching model name")
	}

	// Test finding with different model name
	_, found = store.FindPodForPrefix(ctx, "hello", "model2")
	if found {
		t.Error("Expected not to find prefix with different model name")
	}
}

// TestTTLExpiration tests that prefixes are removed after their TTL expires
func TestTTLExpiration(t *testing.T) {
	ctx := context.Background()
	logger := log.FromContext(ctx)
	ctx = log.IntoContext(ctx, logger)

	config := PrefixStoreConfig{
		MaxEntries:   100,
		MinPrefixLen: 3,
		MaxPrefixLen: 10,
		EntryTTL:     100 * time.Millisecond,
	}

	store := NewPrefixStore(config)
	podName := k8stypes.NamespacedName{
		Name:      "pod1",
		Namespace: "default",
	}

	// Add prefix
	err := store.AddPrefix(ctx, "hello", podName, "model1")
	if err != nil {
		t.Errorf("Failed to add prefix: %v", err)
	}

	// Should find it immediately
	_, found := store.FindPodForPrefix(ctx, "hello", "model1")
	if !found {
		t.Error("Expected to find prefix immediately after adding")
	}

	// Wait for TTL to expire
	time.Sleep(200 * time.Millisecond)

	// Should not find it after TTL expires
	_, found = store.FindPodForPrefix(ctx, "hello", "model1")
	if found {
		t.Error("Expected prefix to be expired after TTL")
	}
}

// TestMaxEntries tests that the store respects the maximum number of entries
func TestMaxEntries(t *testing.T) {
	ctx := context.Background()
	logger := log.FromContext(ctx)
	ctx = log.IntoContext(ctx, logger)

	config := PrefixStoreConfig{
		MaxEntries:   2,
		MinPrefixLen: 3,
		MaxPrefixLen: 10,
		EntryTTL:     1 * time.Hour,
	}

	store := NewPrefixStore(config)
	podName := k8stypes.NamespacedName{
		Name:      "pod1",
		Namespace: "default",
	}

	// Add first prefix
	err := store.AddPrefix(ctx, "prefix1", podName, "model1")
	if err != nil {
		t.Errorf("Failed to add first prefix: %v", err)
	}

	// Add second prefix
	err = store.AddPrefix(ctx, "prefix2", podName, "model1")
	if err != nil {
		t.Errorf("Failed to add second prefix: %v", err)
	}

	// Add third prefix (should cause eviction)
	err = store.AddPrefix(ctx, "prefix3", podName, "model1")
	if err != nil {
		t.Errorf("Failed to add third prefix: %v", err)
	}

	// First prefix should be evicted
	_, found := store.FindPodForPrefix(ctx, "prefix1", "model1")
	if found {
		t.Error("Expected first prefix to be evicted")
	}

	// Second and third prefixes should still be there
	_, found = store.FindPodForPrefix(ctx, "prefix2", "model1")
	if !found {
		t.Error("Expected second prefix to still be present")
	}

	_, found = store.FindPodForPrefix(ctx, "prefix3", "model1")
	if !found {
		t.Error("Expected third prefix to still be present")
	}
}

// TestMaintenanceRoutine tests that the maintenance routine properly cleans up expired entries
func TestMaintenanceRoutine(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	logger := log.FromContext(ctx)
	ctx = log.IntoContext(ctx, logger)

	config := PrefixStoreConfig{
		MaxEntries:   100,
		MinPrefixLen: 3,
		MaxPrefixLen: 10,
		EntryTTL:     100 * time.Millisecond,
	}

	store := NewPrefixStore(config)
	podName := k8stypes.NamespacedName{
		Name:      "pod1",
		Namespace: "default",
	}

	// Add prefix
	err := store.AddPrefix(ctx, "hello", podName, "model1")
	if err != nil {
		t.Errorf("Failed to add prefix: %v", err)
	}

	// Start maintenance routine
	go store.RunMaintenance(ctx)

	// Should find it immediately
	_, found := store.FindPodForPrefix(ctx, "hello", "model1")
	if !found {
		t.Error("Expected to find prefix immediately after adding")
	}

	// Wait for TTL to expire
	time.Sleep(200 * time.Millisecond)

	// Should not find it after TTL expires
	_, found = store.FindPodForPrefix(ctx, "hello", "model1")
	if found {
		t.Error("Expected prefix to be expired after TTL")
	}

	// Clean up
	cancel()
}
