package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const DefaultBaseURL = "https://api.wpengineapi.com/v1"

// Client is a WP Engine API client.
type Client struct {
	BaseURL    string
	username   string
	password   string
	httpClient *http.Client
}

// APIError represents an error returned by the WP Engine API.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("WP Engine API error (status %d): %s", e.StatusCode, e.Message)
}

// NewClient creates a new API client.
func NewClient(username, password string) *Client {
	return &Client{
		BaseURL:    DefaultBaseURL,
		username:   username,
		password:   password,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) doRequest(method, path string, body []byte, queryParams url.Values) ([]byte, error) {
	reqURL, err := url.Parse(c.BaseURL + path)
	if err != nil {
		return nil, err
	}

	if queryParams != nil {
		reqURL.RawQuery = queryParams.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, reqURL.String(), bodyReader)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errData map[string]interface{}
		var errMsg string
		if err := json.Unmarshal(respBody, &errData); err == nil {
			if msg, ok := errData["message"].(string); ok {
				errMsg = msg
			} else if details, ok := errData["details"].(string); ok {
				errMsg = details
			} else {
				errMsg = string(respBody)
			}
		} else {
			errMsg = string(respBody)
		}
		if errMsg == "" {
			errMsg = resp.Status
		}
		return nil, &APIError{StatusCode: resp.StatusCode, Message: errMsg}
	}

	return respBody, nil
}

// GetAccounts retrieves accounts the credentials have access to.
func (c *Client) GetAccounts(limit, offset int) (*AccountsResponse, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}

	body, err := c.doRequest("GET", "/accounts", nil, q)
	if err != nil {
		return nil, err
	}

	var resp AccountsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetSites retrieves sites associated with the accounts.
func (c *Client) GetSites(limit, offset int) (*SitesResponse, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}

	body, err := c.doRequest("GET", "/sites", nil, q)
	if err != nil {
		return nil, err
	}

	var resp SitesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetInstalls retrieves all WordPress installs (environments).
func (c *Client) GetInstalls(limit, offset int) (*InstallsResponse, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}

	body, err := c.doRequest("GET", "/installs", nil, q)
	if err != nil {
		return nil, err
	}

	var resp InstallsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetAllSites retrieves all sites across all pages.
func (c *Client) GetAllSites() ([]Site, error) {
	var allSites []Site
	limit := 100
	offset := 0
	for {
		resp, err := c.GetSites(limit, offset)
		if err != nil {
			return nil, err
		}
		allSites = append(allSites, resp.Results...)
		if len(resp.Results) < limit || len(allSites) >= resp.Count {
			break
		}
		offset += limit
	}
	return allSites, nil
}

// GetAllInstalls retrieves all WordPress installs across all pages.
func (c *Client) GetAllInstalls() ([]Install, error) {
	var allInstalls []Install
	limit := 100
	offset := 0
	for {
		resp, err := c.GetInstalls(limit, offset)
		if err != nil {
			return nil, err
		}
		allInstalls = append(allInstalls, resp.Results...)
		if len(resp.Results) < limit || len(allInstalls) >= resp.Count {
			break
		}
		offset += limit
	}
	return allInstalls, nil
}

// GetInstall retrieves details of a specific install.
func (c *Client) GetInstall(installID string) (*Install, error) {
	body, err := c.doRequest("GET", fmt.Sprintf("/installs/%s", installID), nil, nil)
	if err != nil {
		return nil, err
	}

	var install Install
	if err := json.Unmarshal(body, &install); err != nil {
		return nil, err
	}
	return &install, nil
}

// CreateInstall requests the creation of a new install (environment).
func (c *Client) CreateInstall(req *CreateInstallRequest) (*Install, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest("POST", "/installs", reqBody, nil)
	if err != nil {
		return nil, err
	}

	var install Install
	if err := json.Unmarshal(body, &install); err != nil {
		return nil, err
	}
	return &install, nil
}

// DeleteInstall requests deletion of an install.
func (c *Client) DeleteInstall(installID string) error {
	_, err := c.doRequest("DELETE", fmt.Sprintf("/installs/%s", installID), nil, nil)
	return err
}

// CreateBackup triggers a backup checkpoint for an install.
func (c *Client) CreateBackup(installID string, description string, emails []string) (*Backup, error) {
	if len(emails) == 0 {
		emails = []string{"no-reply@wpengine.com"}
	}
	req := CreateBackupRequest{
		Description:        description,
		NotificationEmails: emails,
	}
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest("POST", fmt.Sprintf("/installs/%s/backups", installID), reqBody, nil)
	if err != nil {
		return nil, err
	}

	var backup Backup
	if err := json.Unmarshal(body, &backup); err != nil {
		return nil, err
	}
	return &backup, nil
}

// GetBackupStatus checks the current status of a specific backup checkpoint.
func (c *Client) GetBackupStatus(installID string, backupID string) (*Backup, error) {
	body, err := c.doRequest("GET", fmt.Sprintf("/installs/%s/backups/%s", installID, backupID), nil, nil)
	if err != nil {
		return nil, err
	}

	var backup Backup
	if err := json.Unmarshal(body, &backup); err != nil {
		return nil, err
	}
	return &backup, nil
}

// PollBackupStatus triggers a go-routine to poll backup status until success, failure, or timeout.
// It returns two channels for streaming backup status updates or errors.
func (c *Client) PollBackupStatus(installID, backupID string, interval time.Duration, timeout time.Duration) (<-chan *Backup, <-chan error) {
	statusChan := make(chan *Backup)
	errChan := make(chan error)

	go func() {
		defer close(statusChan)
		defer close(errChan)

		endTime := time.Now().Add(timeout)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if time.Now().After(endTime) {
					errChan <- fmt.Errorf("timeout waiting for backup %s to complete", backupID)
					return
				}

				backup, err := c.GetBackupStatus(installID, backupID)
				if err != nil {
					errChan <- err
					return
				}

				statusChan <- backup

				switch backup.Status {
				case "completed":
					return
				case "aborted":
					errChan <- fmt.Errorf("backup %s was aborted by WP Engine", backupID)
					return
				}
			}
		}
	}()

	return statusChan, errChan
}
