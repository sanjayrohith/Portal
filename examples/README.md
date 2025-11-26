# Webhook Integration Examples

This directory provides runnable webhook receiver servers and realistic test
fixtures for three of the most widely used webhook providers:
**Stripe**, **GitHub**, and **Slack**.

Each example demonstrates:
1. Validating provider cryptographic signatures (`HMAC-SHA256`).
2. Replay attack protection via timestamp verification.
3. Exposing the local listener securely over the internet using **Portal**.
4. Replaying captured webhook payloads using Portal's built-in web inspector at
   `http://127.0.0.1:4040`.

---

## 1. Stripe Webhooks

Stripe signs webhook events using the `Stripe-Signature` header, which contains
a timestamp (`t=...`) and an HMAC-SHA256 signature (`v1=...`).

### Run the Receiver

```bash
# Set your webhook signing secret (obtained from Stripe Dashboard)
export STRIPE_WEBHOOK_SECRET="whsec_test_secret_12345"

# Run the receiver on port 8081
go run ./examples/stripe/main.go
```

### Expose with Portal

In a separate terminal, expose your local service on a persistent subdomain:

```bash
portal http 8081 --subdomain stripe-dev
```

Your public webhook endpoint will be:
`https://stripe-dev.yourdomain.com/webhook/stripe`

### Test with the Fixture

Send the fixture payload using `curl` and a calculated signature:

```bash
TIMESTAMP=$(date +%s)
PAYLOAD=$(cat ./examples/stripe/test_payload.json)
SIG=$(echo -n "${TIMESTAMP}.${PAYLOAD}" | openssl dgst -sha256 -hmac "whsec_test_secret_12345" | sed 's/^.* //')

curl -X POST https://stripe-dev.yourdomain.com/webhook/stripe \
  -H "Content-Type: application/json" \
  -H "Stripe-Signature: t=${TIMESTAMP},v1=${SIG}" \
  -d "$PAYLOAD"
```

---

## 2. GitHub Webhooks

GitHub signs payloads using the `X-Hub-Signature-256` header formatted as
`sha256=<hex_hmac>`.

### Run the Receiver

```bash
export GITHUB_WEBHOOK_SECRET="ghsec_test_secret_67890"

# Run receiver on port 8082
go run ./examples/github/main.go
```

### Expose with Portal

```bash
portal http 8082 --subdomain github-dev
```

Your public webhook endpoint will be:
`https://github-dev.yourdomain.com/webhook/github`

### Test with the Fixture

```bash
PAYLOAD=$(cat ./examples/github/test_payload.json)
SIG=$(echo -n "$PAYLOAD" | openssl dgst -sha256 -hmac "ghsec_test_secret_67890" | sed 's/^.* //')

curl -X POST https://github-dev.yourdomain.com/webhook/github \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: issues" \
  -H "X-GitHub-Delivery: 72d3162e-cc78-11e3-81ab-4c9367dc0958" \
  -H "X-Hub-Signature-256: sha256=${SIG}" \
  -d "$PAYLOAD"
```

---

## 3. Slack Events & Slash Commands

Slack signs incoming event callbacks using `X-Slack-Signature` and
`X-Slack-Request-Timestamp`. The signature base string is
`v0:${TIMESTAMP}:${BODY}`.

### Run the Receiver

```bash
export SLACK_SIGNING_SECRET="slack_signing_secret_998877"

# Run receiver on port 8083
go run ./examples/slack/main.go
```

### Expose with Portal

```bash
portal http 8083 --subdomain slack-dev
```

Your public endpoint will be:
`https://slack-dev.yourdomain.com/webhook/slack`

### Test with the Fixture

```bash
TIMESTAMP=$(date +%s)
PAYLOAD=$(cat ./examples/slack/test_payload.json)
BASE="v0:${TIMESTAMP}:${PAYLOAD}"
SIG=$(echo -n "$BASE" | openssl dgst -sha256 -hmac "slack_signing_secret_998877" | sed 's/^.* //')

curl -X POST https://slack-dev.yourdomain.com/webhook/slack \
  -H "Content-Type: application/json" \
  -H "X-Slack-Request-Timestamp: ${TIMESTAMP}" \
  -H "X-Slack-Signature: v0=${SIG}" \
  -d "$PAYLOAD"
```

---

## 4. Debugging & Replaying with Web Inspector

When running `portal http <port>`, open your browser to:

👉 **`http://127.0.0.1:4040`**

### Why Portal Inspector is ideal for Webhooks:

- **Full Header & Body Inspection**: Review exact raw bodies, content types, and signature headers delivered by external providers.
- **Instant Replay**: Instead of triggering another real checkout or pushing another commit, click the **Replay** button in the inspector UI. Portal re-transmits the captured request directly to your local handler on localhost, speeding up debugging loops 10x.
- **In-Memory Privacy**: Webhook tokens and sensitive customer data remain strictly in loopback memory (circular ring buffer) and are never written to disk.
