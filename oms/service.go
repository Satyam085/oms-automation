package oms

import (
	"encoding/json"
	"fmt"
	"io"
	"log"

	"oms-automtion/config"
	"oms-automtion/models"
)

// FetchPendingOutages fetches all pending outages using pagination.
func (c *Client) FetchPendingOutages() ([]models.Outage, error) {
	var all []models.Outage
	offset := 0

	for {
		payload := models.PendingRequest{
			FilteredData: []models.FilteredData{{
				ID: 14, Field: "outage_occur_at", Type: "date",
				Operator:  "between",
				Value:     "2026-01-02 to 2026-02-01",
				FromValue: "2026-01-02",
				ToValue:   "2026-02-01",
			}},
			Offset: offset,
			Limit:  config.PageSize,
		}

		body, _ := json.Marshal(payload)
		req, err := c.NewAPIRequest("POST", config.BaseURL+"/reason/pending", body)
		if err != nil {
			return nil, err
		}

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch pending: %w", err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("pending returned %d: %s", resp.StatusCode, respBody)
		}

		var pr models.PendingResponse
		if err := json.Unmarshal(respBody, &pr); err != nil {
			return nil, fmt.Errorf("unmarshal pending: %w", err)
		}

		all = append(all, pr.Data...)
		log.Printf("  [Fetch] offset=%d got=%d total=%d", offset, len(pr.Data), pr.TotalRecords)

		if offset+config.PageSize >= pr.TotalRecords || len(pr.Data) == 0 {
			break
		}
		offset += config.PageSize
	}
	return all, nil
}

// FetchLocIDs extracts loc_ids from the GeoJSON response for a specific outage.
func (c *Client) FetchLocIDs(outageID string) ([]int, error) {
	url := fmt.Sprintf("%s/reason/outage/%s", config.BaseURL, outageID)

	req, err := c.NewAPIRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch detail %s: %w", outageID, err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("detail %s returned %d: %s", outageID, resp.StatusCode, respBody)
	}

	var detail models.ReasonDetailResponse
	if err := json.Unmarshal(respBody, &detail); err != nil {
		return nil, fmt.Errorf("unmarshal detail %s: %w", outageID, err)
	}

	// Walk: feederPointGeoJson[*][*].row_to_json.features[*].properties.id
	var locIDs []int
	for _, outer := range detail.Data.FeederPointGeoJson {
		for _, inner := range outer {
			for _, feat := range inner.RowToJSON.Features {
				if feat.Properties.ID != 0 {
					locIDs = append(locIDs, feat.Properties.ID)
				}
			}
		}
	}
	return locIDs, nil
}

// SubmitReason posts the selected reason and location for an outage.
func (c *Client) SubmitReason(outageID string, locID int, reasonID int) error {
	url := fmt.Sprintf("%s/reason/outage/%s", config.BaseURL, outageID)

	payload := []models.ReasonPayloadItem{{LocID: locID, ReasonID: reasonID}}
	body, _ := json.Marshal(payload)

	req, err := c.NewAPIRequest("POST", url, body)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("submit %s: %w", outageID, err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("submit %s returned %d: %s", outageID, resp.StatusCode, respBody)
	}
	return nil
}
