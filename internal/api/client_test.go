package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetAccounts(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/accounts" {
			t.Errorf("expected path /v1/accounts, got %s", r.URL.Path)
		}
		
		username, password, ok := r.BasicAuth()
		if !ok || username != "testuser" || password != "testpass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		resp := AccountsResponse{
			Count: 1,
			Results: []Account{
				{ID: "acc-uuid-1", Name: "Test Account", Nickname: "test-nickname"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Instantiate client pointing to mock server
	client := NewClient("testuser", "testpass")
	client.BaseURL = server.URL + "/v1"

	res, err := client.GetAccounts(0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Count != 1 || len(res.Results) != 1 {
		t.Errorf("expected count 1 and 1 result, got count %d and len %d", res.Count, len(res.Results))
	}

	if res.Results[0].Name != "Test Account" {
		t.Errorf("expected name 'Test Account', got '%s'", res.Results[0].Name)
	}
}

func TestPollBackupStatus(t *testing.T) {
	pollCount := 0

	// Mock server that returns initiated first, then completed
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/v1/installs/inst-1/backups/b-1" {
			pollCount++
			var status string
			if pollCount == 1 {
				status = "initiated"
			} else {
				status = "completed"
			}

			resp := Backup{
				ID:          "b-1",
				Status:      status,
				Description: "pre-update backup",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient("test", "test")
	client.BaseURL = server.URL + "/v1"

	statusChan, errChan := client.PollBackupStatus("inst-1", "b-1", 50*time.Millisecond, 2*time.Second)

	var lastBackup *Backup
	for {
		select {
		case b, ok := <-statusChan:
			if !ok {
				goto done
			}
			lastBackup = b
		case err := <-errChan:
			if err != nil {
				t.Fatalf("unexpected polling error: %v", err)
			}
		}
	}
done:

	if pollCount != 2 {
		t.Errorf("expected 2 status polls, got %d", pollCount)
	}

	if lastBackup == nil || lastBackup.Status != "completed" {
		t.Errorf("expected last backup status to be completed, got %v", lastBackup)
	}
}
