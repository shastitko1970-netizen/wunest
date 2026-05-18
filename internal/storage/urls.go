package storage

import "strings"

// ObjectKeyFromPublicURL extracts the MinIO object key from a public URL
// served under pathPrefix (e.g. "/images/avatars/").
func ObjectKeyFromPublicURL(publicURL, pathPrefix string) (string, bool) {
	if publicURL == "" || pathPrefix == "" {
		return "", false
	}
	i := strings.Index(publicURL, pathPrefix)
	if i < 0 {
		return "", false
	}
	key := publicURL[i+len(pathPrefix):]
	if q := strings.IndexAny(key, "?#"); q >= 0 {
		key = key[:q]
	}
	if key == "" {
		return "", false
	}
	return key, true
}

// ObjectKeysFromPublicURLs collects keys for any known /images/* bucket path.
func ObjectKeysFromPublicURLs(urls ...string) []string {
	prefixes := []string{
		"/images/avatars/",
		"/images/attachments/",
		"/images/backgrounds/",
	}
	seen := map[string]struct{}{}
	var keys []string
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		for _, p := range prefixes {
			if k, ok := ObjectKeyFromPublicURL(u, p); ok {
				if _, dup := seen[k]; dup {
					continue
				}
				seen[k] = struct{}{}
				keys = append(keys, k)
			}
		}
	}
	return keys
}
