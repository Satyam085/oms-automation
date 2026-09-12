# oms-automation

Submits outage reasons in bulk to the DGVCL OMS (`omsapi.geourja.com`): logs in,
fetches pending outages, picks a reason from the outage duration, and submits it
against a pole on the feeder.

## Setup

Copy `.env.example` to `.env` and fill in real values — **the example file holds
placeholders only, never commit credentials**:

    PASSCODE=<6 digits, unlocks profile 1 in the web UI>
    OMS_COMPANY_NAME=DGVCL
    OMS_EMP_NO=<employee number>
    OMS_PASSWORD=<OMS password>
    OMS_APP_NAME=SFMS-Web

Extra users are `PASSCODE_2`, `OMS_EMP_NO_2`, … up to `_10`.

## Run

    go run .                        # CLI, all pending outages, profile 1
    go run . -limit 20 -profile 2   # first 20 outages as profile 2
    go run . -rules 21,20,31        # only these reason IDs
    go run . -server                # web UI on :8080 (also RUN_MODE=server)

    docker build -t oms-automation . && docker run --env-file .env -p 8080:8080 oms-automation

## Duration → reason

Buckets live in `config.DurationRules`; `ruleFor` in `outage_automation.go`
applies them plus the overrides (>8h → #25 No Cause found, KUMBHIYA >6h → #25).

    go test ./...
