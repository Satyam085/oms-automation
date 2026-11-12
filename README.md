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

Extra users are `PASSCODE_2`, `OMS_EMP_NO_2`, … up to `_10`. There are no
built-in fallbacks: a profile without `OMS_EMP_NO`/`OMS_PASSWORD` fails to log in,
and the web UI refuses to start without a 6-digit `PASSCODE`.

## Rate limits

OMS answers bursts with 429 Too Many Requests. The client spaces every request
at least `OMS_REQUEST_GAP_MS` apart (default 1s), doubles that gap after each
429 for the rest of the run, and waits out the 429 itself (Retry-After, or
30s/1m/2m… backoff) up to `OMS_RATE_LIMIT_WAIT`.

To keep the request count down, `/reason/pending` takes the whole queue in one
call (`OMS_PAGE_SIZE`, default 500 — it costs about the same as a call for 10),
and a feeder's pole list (1 MB+ for a big feeder) is fetched once per run, not
once per outage: every sustained fault on that feeder reuses it.

## Run

    go run .                        # CLI, all pending outages, profile 1
    go run . -limit 20 -profile 2   # first 20 outages as profile 2
    go run . -rules 21,20,31        # only these reason IDs
    go run . -server                # web UI on :8080 (also RUN_MODE=server)

    docker build -t oms-automation . && docker run --env-file .env -p 8080:8080 oms-automation

## Duration → reason

Buckets live in `config.DurationRules`; `ruleFor` in `outage_automation.go`
applies them plus the overrides: KUMBHIYA/KHAKHAR 14.5–16.5h → #28 Relay Problems,
then KUMBHIYA >6h → #25, then anything else >8h → #25 No Cause found.

    go test ./...
