package api

import "time"

// Account represents a WP Engine account.
type Account struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Nickname string `json:"nickname"`
}

// AccountsResponse represents the paginated response for listing accounts.
type AccountsResponse struct {
	Count    int       `json:"count"`
	Next     string    `json:"next"`
	Previous string    `json:"previous"`
	Results  []Account `json:"results"`
}

// SiteAccount represents the account information nested in site structures.
type SiteAccount struct {
	ID string `json:"id"`
}

// Site represents a WP Engine site grouping multiple environments.
type Site struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Account SiteAccount `json:"account"`
}

// SitesResponse represents the paginated response for listing sites.
type SitesResponse struct {
	Count    int    `json:"count"`
	Next     string `json:"next"`
	Previous string `json:"previous"`
	Results  []Site `json:"results"`
}

// InstallAccount represents the account information nested inside install structures.
type InstallAccount struct {
	ID string `json:"id"`
}

// InstallSite represents the site information nested inside install structures.
type InstallSite struct {
	ID string `json:"id"`
}

// Install represents an environment (install) in WP Engine.
type Install struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	CNAME         string         `json:"cname"`
	Environment   string         `json:"environment"` // production, staging, development
	PrimaryDomain string         `json:"primary_domain"`
	Status        string         `json:"status"` // active, pending, etc.
	Account       InstallAccount `json:"account"`
	Site          InstallSite    `json:"site"`
	PHPVersion    string         `json:"php_version,omitempty"`
}

// InstallsResponse represents the paginated response for listing installs.
type InstallsResponse struct {
	Count    int       `json:"count"`
	Next     string    `json:"next"`
	Previous string    `json:"previous"`
	Results  []Install `json:"results"`
}

// CreateInstallRequest is the request payload to spin up a new environment.
type CreateInstallRequest struct {
	Name        string `json:"name"`
	AccountID   string `json:"account_id"`
	SiteID      string `json:"site_id,omitempty"`
	Environment string `json:"environment,omitempty"` // production, staging, development
}

// Backup represents a backup checkpoint.
type Backup struct {
	ID          string    `json:"id"`
	Status      string    `json:"status"` // requested, initiated, completed, aborted
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
}

// CreateBackupRequest is the request payload to trigger a backup.
type CreateBackupRequest struct {
	Description string `json:"description"`
}
