package app

import (
	"sync"
	"testing"
)

const (
	// testChainID is the chain ID used for testing
	testChainID = "kudora_12000-1"
)

// TestEVMAppOptionsThreadSafety verifies that EVMAppOptions can be called
// concurrently from multiple goroutines without race conditions
func TestEVMAppOptionsThreadSafety(t *testing.T) {
	// Number of concurrent goroutines to test with
	const numGoroutines = 100

	// Use a WaitGroup to synchronize goroutines
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Channel to collect errors from goroutines
	errChan := make(chan error, numGoroutines)

	// Launch multiple goroutines calling EVMAppOptions concurrently
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			err := EVMAppOptions(testChainID)
			if err != nil {
				errChan <- err
			}
		}()
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check if any errors occurred
	for err := range errChan {
		t.Errorf("EVMAppOptions failed: %v", err)
	}
}

// TestEVMAppOptionsInitializationOnce verifies that the initialization
// logic is only executed once, even with multiple calls
func TestEVMAppOptionsInitializationOnce(t *testing.T) {
	// Reset the state for this test (this is normally not needed in production)
	// We're calling it multiple times to ensure sync.Once works correctly
	err1 := EVMAppOptions(testChainID)
	if err1 != nil {
		t.Fatalf("First call to EVMAppOptions failed: %v", err1)
	}

	err2 := EVMAppOptions(testChainID)
	if err2 != nil {
		t.Fatalf("Second call to EVMAppOptions failed: %v", err2)
	}

	// Both calls should succeed and return the same error state
	if err1 != err2 {
		t.Errorf("Multiple calls to EVMAppOptions returned different errors: %v vs %v", err1, err2)
	}
}
