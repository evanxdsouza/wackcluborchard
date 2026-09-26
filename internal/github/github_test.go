package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"testing"
)

func TestParseDockerfile(t *testing.T) {
	in := ParseDockerfile(`FROM node:22-alpine AS base
WORKDIR /app
ARG NODE_ENV=production
FROM base AS deps
COPY package.json ./
RUN npm ci
FROM base AS runner
ENV PORT=3000 \
    HOSTNAME=0.0.0.0
EXPOSE 3000
CMD ["node", "server.js"]
`)
	if !reflect.DeepEqual(in.Stages, []string{"base", "deps", "runner"}) || in.Target != "runner" || in.Port != 3000 {
		t.Fatalf("%+v", in)
	}
	if !reflect.DeepEqual(in.Env, []string{"NODE_ENV", "PORT"}) {
		t.Fatalf("env: %v", in.Env)
	}
}

func TestVerifyWebhook(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	m := hmac.New(sha256.New, []byte("s3cret"))
	m.Write(body)
	sig := "sha256=" + hex.EncodeToString(m.Sum(nil))
	if !VerifyWebhook("s3cret", body, sig) {
		t.Fatal("valid signature rejected")
	}
	if VerifyWebhook("other", body, sig) || VerifyWebhook("", body, sig) || VerifyWebhook("s3cret", body, "sha1=x") {
		t.Fatal("invalid signature accepted")
	}
}
