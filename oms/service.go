package oms

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"oms-automtion/config"
	"oms-automtion/models"
)

var ErrNoGeoLocation = errors.New("no geo location found")

// FetchPendingPage fetches one page of pending outages and the overall pending
// count. Callers clear what they get and fetch again rather than paginating:
// cleared outages drop off the list and fresh ones move up into its place.
// offset only needs to move past outages the caller saw but could not clear.
func (c *Client) FetchPendingPage(offset, limit int) ([]models.Outage, int, error) {
	url := fmt.Sprintf("%s/reason/pending", config.BaseURL)
	reqBody := models.PendingRequest{
		FilteredData: []models.FilteredData{},
		Offset:       offset,
		Limit:        limit,
	}

	body, _ := json.Marshal(reqBody)
	req, err := c.NewAPIRequest("POST", url, body)
	if err != nil {
		return nil, 0, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch pending: %w", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, 0, fmt.Errorf("pending returned %d: %s", resp.StatusCode, respBody)
	}

	var pr models.PendingResponse
	if err := json.Unmarshal(respBody, &pr); err != nil {
		return nil, 0, fmt.Errorf("unmarshal pending: %w", err)
	}

	c.Log.Printf("  [Fetch] offset=%d got=%d total=%d", offset, len(pr.Data), pr.TotalRecords)
	return pr.Data, pr.TotalRecords, nil
}

// FetchLocIDs extracts loc_ids from the GeoJSON response for a specific outage.
func (c *Client) FetchLocIDs(outageID string, feederID int) ([]int, error) {
	url := fmt.Sprintf("%s/reason/%d/%s", config.BaseURL, feederID, outageID)

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

	if strings.Contains(string(respBody), "no_geo_location") {
		return nil, ErrNoGeoLocation
	}

	var detail models.ReasonDetailResponse
	if err := json.Unmarshal(respBody, &detail); err != nil {
		return nil, fmt.Errorf("unmarshal detail %s: %w", outageID, err)
	}

	// Walk: feederPointGeoJson[*][*].row_to_json.features[*].properties.id
	// Filter: only select poles where hlt == "HT Pole"
	// Note: feederPointGeoJson contains mixed types (arrays and objects), so we parse each element
	var locIDs []int
	for _, rawElem := range detail.Data.FeederPointGeoJson {
		// Try to unmarshal as array of RowToJSONWrapper
		var wrappers []models.RowToJSONWrapper
		if err := json.Unmarshal(rawElem, &wrappers); err != nil {
			// Skip non-array elements (like the metadata object)
			continue
		}

		// Extract pole IDs from this array
		for _, wrapper := range wrappers {
			for _, feat := range wrapper.RowToJSON.Features {
				// Only include HT Poles
				if feat.Properties.Hlt == "HT Pole" && feat.Properties.ID != 0 {
					locIDs = append(locIDs, feat.Properties.ID)
				}
			}
		}
	}

	if len(locIDs) == 0 {
		return nil, ErrNoGeoLocation
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

// SubmitNoGeoReason posts the default general maintenance reason when no geo location is available.
func (c *Client) SubmitNoGeoReason(outageID string) error {
	url := fmt.Sprintf("%s/reason/outage/%s", config.BaseURL, outageID)

	payload := []models.NoGeoReasonPayloadItem{{
		ReasonID:   "44",
		ReasonName: "General Maintenance",
	}}
	body, _ := json.Marshal(payload)

	req, err := c.NewAPIRequest("POST", url, body)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("submit no_geo %s: %w", outageID, err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("submit no_geo %s returned %d: %s", outageID, resp.StatusCode, respBody)
	}
	return nil
}
