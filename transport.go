package kagi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
)

const authorizationScheme = "Bearer"

func (c *Client) newRequest(ctx context.Context, method, endpointPath string, query url.Values, body any) (*http.Request, error) {
	if ctx == nil {
		return nil, errors.New("kagi: nil context")
	}

	var requestBody io.Reader
	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, err
		}
		requestBody = &buf
	}

	req, err := http.NewRequestWithContext(ctx, method, c.endpointURL(endpointPath, query), requestBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", authorizationScheme+" "+c.apiKey)
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	return c.httpClient.Do(req)
}

func (c *Client) endpointURL(endpointPath string, query url.Values) string {
	endpoint := c.baseURL.JoinPath(endpointPath)
	if len(query) > 0 {
		endpoint.RawQuery = query.Encode()
	}
	return endpoint.String()
}
