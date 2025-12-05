package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Client handles communication with SleakOps Core API
type Client struct {
	BaseURL    string
	Email      string
	Password   string
	Account    string
	HTTPClient *http.Client
	accessToken string
}

// LoginResponse represents the response from /api/login
type LoginResponse struct {
	Access       string `json:"access"`
	Refresh      string `json:"refresh"`
	User         User   `json:"user"`
	MFAEnabled   bool   `json:"mfa_enabled"`
	Subscription string `json:"subscription"`
}

// User represents user data in login response
type User struct {
	PK        string `json:"pk"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// TokenRefreshResponse represents the response from /api/token/refresh/
type TokenRefreshResponse struct {
	Access           string `json:"access"`
	AccessExpiration string `json:"access_expiration"`
}

// NewClient creates a new SleakOps API client
func NewClient(baseURL, email, password, account string) (*Client, error) {
	// Create cookie jar to store cookies from login
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}

	client := &Client{
		BaseURL:  baseURL,
		Email:    email,
		Password: password,
		Account:  account,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
		},
	}

	// Authenticate on client creation
	if err := client.authenticate(context.Background()); err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	return client, nil
}

// authenticate performs login and stores cookies
func (c *Client) authenticate(ctx context.Context) error {
	loginURL := fmt.Sprintf("%s/api/login", c.BaseURL)

	payload := map[string]string{
		"email":    c.Email,
		"password": c.Password,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal login payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", loginURL, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	tflog.Debug(ctx, "Authenticating with SleakOps Core", map[string]any{
		"url":   loginURL,
		"email": c.Email,
	})

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var loginResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return fmt.Errorf("failed to decode login response: %w", err)
	}

	// Store access token for reference (cookies are stored automatically by jar)
	c.accessToken = loginResp.Access

	tflog.Info(ctx, "Successfully authenticated with SleakOps Core", map[string]any{
		"user_email": loginResp.User.Email,
		"user_id":    loginResp.User.PK,
	})

	return nil
}

// refreshToken refreshes the access token using the refresh token from cookies
func (c *Client) refreshToken(ctx context.Context) error {
	refreshURL := fmt.Sprintf("%s/api/token/refresh/", c.BaseURL)

	req, err := http.NewRequestWithContext(ctx, "POST", refreshURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create refresh request: %w", err)
	}

	tflog.Debug(ctx, "Refreshing access token", map[string]any{"url": refreshURL})

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("token refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var refreshResp TokenRefreshResponse
	if err := json.NewDecoder(resp.Body).Decode(&refreshResp); err != nil {
		return fmt.Errorf("failed to decode refresh response: %w", err)
	}

	// Update stored access token
	c.accessToken = refreshResp.Access

	tflog.Debug(ctx, "Successfully refreshed access token", map[string]any{
		"expiration": refreshResp.AccessExpiration,
	})

	return nil
}

// doRequest performs an HTTP request with automatic token refresh on 401
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	reqURL := fmt.Sprintf("%s%s", c.BaseURL, path)

	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if c.Account != "" {
		req.Header.Set("Account", c.Account)
	}

	tflog.Debug(ctx, "Making API request", map[string]any{
		"method":  method,
		"url":     reqURL,
		"account": c.Account,
	})

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// Handle 401 Unauthorized - attempt token refresh
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()

		tflog.Debug(ctx, "Received 401, attempting token refresh")

		if err := c.refreshToken(ctx); err != nil {
			return nil, fmt.Errorf("token refresh failed: %w", err)
		}

		// Retry the original request
		if body != nil {
			jsonBody, _ := json.Marshal(body)
			reqBody = bytes.NewBuffer(jsonBody)
		}

		req, err = http.NewRequestWithContext(ctx, method, reqURL, reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to create retry request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		if c.Account != "" {
			req.Header.Set("Account", c.Account)
		}

		tflog.Debug(ctx, "Retrying request after token refresh")

		resp, err = c.HTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("retry request failed: %w", err)
		}
	}

	return resp, nil
}

// Get performs a GET request
func (c *Client) Get(ctx context.Context, path string) (*http.Response, error) {
	return c.doRequest(ctx, "GET", path, nil)
}

// Post performs a POST request
func (c *Client) Post(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	return c.doRequest(ctx, "POST", path, body)
}

// Patch performs a PATCH request
func (c *Client) Patch(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	return c.doRequest(ctx, "PATCH", path, body)
}

// Delete performs a DELETE request
func (c *Client) Delete(ctx context.Context, path string) (*http.Response, error) {
	return c.doRequest(ctx, "DELETE", path, nil)
}

// ParseBaseURL ensures the base URL is properly formatted
func ParseBaseURL(rawURL string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("base URL cannot be empty")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	// Ensure scheme is present
	if u.Scheme == "" {
		return "", fmt.Errorf("base URL must include scheme (http:// or https://)")
	}

	// Remove trailing slash
	baseURL := u.Scheme + "://" + u.Host
	if u.Path != "" && u.Path != "/" {
		baseURL += u.Path
	}

	return baseURL, nil
}
