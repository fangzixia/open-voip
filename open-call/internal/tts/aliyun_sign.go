package tts

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

func aliyunRPCSign(method, secret string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var pairs []string
	for _, k := range keys {
		pairs = append(pairs, percentEncode(k)+"="+percentEncode(params[k]))
	}
	canonical := strings.Join(pairs, "&")
	stringToSign := method + "&" + percentEncode("/") + "&" + percentEncode(canonical)
	mac := hmac.New(sha1.New, []byte(secret+"&"))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func percentEncode(s string) string {
	enc := url.QueryEscape(s)
	enc = strings.ReplaceAll(enc, "+", "%20")
	enc = strings.ReplaceAll(enc, "*", "%2A")
	enc = strings.ReplaceAll(enc, "%7E", "~")
	return enc
}

func aliyunTimestampUTC() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05Z")
}

func aliyunMetaSignedURL(metaURL, accessKeyID, accessKeySecret string) (string, error) {
	params := map[string]string{
		"Action":           "CreateToken",
		"Version":          "2019-02-28",
		"Format":           "JSON",
		"RegionId":         "cn-shanghai",
		"AccessKeyId":      accessKeyID,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureVersion": "1.0",
		"SignatureNonce":   fmt.Sprintf("%d", time.Now().UnixNano()),
		"Timestamp":        aliyunTimestampUTC(),
	}
	params["Signature"] = aliyunRPCSign("GET", accessKeySecret, params)
	u, err := url.Parse(strings.TrimRight(metaURL, "/"))
	if err != nil {
		return "", err
	}
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
