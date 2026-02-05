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
			FilteredData: []models.FilteredData{}, // Empty filter to match browser behavior
			Offset:       offset,
			Limit:        config.PageSize,
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

		// DEBUG: Print raw response for first page
		if offset == 0 {
			log.Printf("\n[DEBUG] Raw API Response (first page):\n%s\n", string(respBody))
		}

		var pr models.PendingResponse
		if err := json.Unmarshal(respBody, &pr); err != nil {
			return nil, fmt.Errorf("unmarshal pending: %w", err)
		}

		all = append(all, pr.Data...)
		log.Printf("  [Fetch] offset=%d got=%d total=%d", offset, len(pr.Data), pr.TotalRecords)

		// DEBUG: Only fetch first page for now
		log.Println("  [DEBUG] Stopping after first page for debugging")
		break

		// if offset+config.PageSize >= pr.TotalRecords || len(pr.Data) == 0 {
		// 	break
		// }
		// offset += config.PageSize
		// 
		// // Rate limiting: delay between pagination requests
		// time.Sleep(time.Duration(config.DelayBetweenPages) * time.Millisecond)
	}
	return all, nil
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
	return locIDs, nil
}

// SubmitReason posts the selected reason and location for an outage.
func (c *Client) SubmitReason(outageID string, feederID int, locID int, reasonID int) error {
	url := fmt.Sprintf("%s/reason/%d/%s", config.BaseURL, feederID, outageID)

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
