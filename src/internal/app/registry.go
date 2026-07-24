package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (a *App) publicRegistryNewerVersion(ctx context.Context, image string) (string, string, bool) {
	registry, repository, reference := parseImageReference(image)
	current, ok := parseSemanticTag(reference)
	if !ok || repository == "" {
		return "", "", false
	}
	tagsURL := "https://" + registry + "/v2/" + repository + "/tags/list?n=1000"
	tags, challenge, checked := a.registryTagsRequest(ctx, tagsURL, "")
	if !checked && challenge != "" {
		token := a.registryBearerToken(ctx, challenge, repository)
		if token != "" {
			tags, _, checked = a.registryTagsRequest(ctx, tagsURL, token)
		}
	}
	if !checked {
		return "", "", false
	}
	best, bestTag := current, ""
	for _, tag := range tags {
		version, valid := parseSemanticTag(tag)
		if valid && compareSemanticVersion(version, best) > 0 {
			best, bestTag = version, tag
		}
	}
	if bestTag == "" {
		return "", "", true
	}
	return bestTag, a.publicRegistryDigest(ctx, imageWithTag(image, bestTag)), true
}

type semanticVersion struct {
	major int
	minor int
	patch int
}

func parseSemanticTag(tag string) (semanticVersion, bool) {
	value := strings.TrimSpace(tag)
	if strings.HasPrefix(value, "v") || strings.HasPrefix(value, "V") {
		value = value[1:]
	}
	parts := strings.Split(value, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return semanticVersion{}, false
	}
	values := [3]int{}
	for index, part := range parts {
		if part == "" {
			return semanticVersion{}, false
		}
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return semanticVersion{}, false
		}
		values[index] = number
	}
	return semanticVersion{major: values[0], minor: values[1], patch: values[2]}, true
}

func compareSemanticVersion(left, right semanticVersion) int {
	leftParts := [...]int{left.major, left.minor, left.patch}
	rightParts := [...]int{right.major, right.minor, right.patch}
	for index := range leftParts {
		if leftParts[index] > rightParts[index] {
			return 1
		}
		if leftParts[index] < rightParts[index] {
			return -1
		}
	}
	return 0
}

func imageWithTag(image, tag string) string {
	image = strings.TrimSpace(image)
	if at := strings.LastIndex(image, "@"); at >= 0 {
		image = image[:at]
	} else if colon := strings.LastIndex(image, ":"); colon > strings.LastIndex(image, "/") {
		image = image[:colon]
	}
	return image + ":" + tag
}

func (a *App) registryTagsRequest(ctx context.Context, tagsURL, token string) ([]string, string, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tagsURL, nil)
	if err != nil {
		return nil, "", false
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, resp.Header.Get("WWW-Authenticate"), false
	}
	if resp.StatusCode/100 != 2 {
		return nil, "", false
	}
	var result struct {
		Tags []string `json:"tags"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result) != nil {
		return nil, "", false
	}
	return result.Tags, "", true
}

func (a *App) registryBearerToken(ctx context.Context, challenge, repository string) string {
	realm, service, scope := parseBearerChallenge(challenge)
	if realm == "" {
		return ""
	}
	values := url.Values{}
	if service != "" {
		values.Set("service", service)
	}
	if scope == "" {
		scope = "repository:" + repository + ":pull"
	}
	values.Set("scope", scope)
	separator := "?"
	if strings.Contains(realm, "?") {
		separator = "&"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realm+separator+values.Encode(), nil)
	if err != nil {
		return ""
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var token struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if resp.StatusCode/100 != 2 || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&token) != nil {
		return ""
	}
	if token.Token != "" {
		return token.Token
	}
	return token.AccessToken
}

func (a *App) publicRegistryDigest(ctx context.Context, image string) string {
	registry, repository, reference := parseImageReference(image)
	if repository == "" {
		return ""
	}
	manifestURL := "https://" + registry + "/v2/" + repository + "/manifests/" + url.PathEscape(reference)
	digest, challenge := a.registryManifestRequest(ctx, manifestURL, "")
	if digest != "" || challenge == "" {
		return digest
	}
	realm, service, scope := parseBearerChallenge(challenge)
	if realm == "" {
		return ""
	}
	values := url.Values{}
	if service != "" {
		values.Set("service", service)
	}
	if scope == "" {
		scope = "repository:" + repository + ":pull"
	}
	values.Set("scope", scope)
	separator := "?"
	if strings.Contains(realm, "?") {
		separator = "&"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realm+separator+values.Encode(), nil)
	if err != nil {
		return ""
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var token struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if resp.StatusCode/100 != 2 || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&token) != nil {
		return ""
	}
	if token.Token == "" {
		token.Token = token.AccessToken
	}
	digest, _ = a.registryManifestRequest(ctx, manifestURL, token.Token)
	return digest
}

func (a *App) registryManifestRequest(ctx context.Context, manifestURL, token string) (string, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, manifestURL, nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ", "))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 == 2 {
		return strings.TrimSpace(resp.Header.Get("Docker-Content-Digest")), ""
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return "", resp.Header.Get("WWW-Authenticate")
	}
	return "", ""
}

func parseImageReference(image string) (string, string, string) {
	image = strings.TrimSpace(image)
	reference := "latest"
	if at := strings.LastIndex(image, "@"); at >= 0 {
		reference, image = image[at+1:], image[:at]
	} else if colon := strings.LastIndex(image, ":"); colon > strings.LastIndex(image, "/") {
		reference, image = image[colon+1:], image[:colon]
	}
	parts := strings.Split(image, "/")
	registry := "registry-1.docker.io"
	if len(parts) > 1 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") || parts[0] == "localhost") {
		registry, parts = parts[0], parts[1:]
	}
	if registry == "docker.io" || registry == "index.docker.io" {
		registry = "registry-1.docker.io"
	}
	if registry == "registry-1.docker.io" && len(parts) == 1 {
		parts = append([]string{"library"}, parts...)
	}
	return registry, strings.Join(parts, "/"), reference
}

func parseBearerChallenge(challenge string) (string, string, string) {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(challenge)), "bearer ") {
		return "", "", ""
	}
	values := map[string]string{}
	for _, part := range strings.Split(strings.TrimSpace(challenge[len("Bearer "):]), ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok {
			values[strings.ToLower(key)] = strings.Trim(strings.TrimSpace(value), `"`)
		}
	}
	return values["realm"], values["service"], values["scope"]
}
