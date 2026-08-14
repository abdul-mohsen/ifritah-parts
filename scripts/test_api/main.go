package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func main() {
	base := "http://localhost:8080"
	pass, fail := 0, 0

	// 1. Health check
	test := func(name, urlPath string, check func(map[string]any) bool) {
		resp, err := http.Get(base + urlPath)
		if err != nil {
			fmt.Printf("FAIL %s: %v\n", name, err)
			fail++
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			fmt.Printf("FAIL %s: HTTP %d — %s\n", name, resp.StatusCode, string(body))
			fail++
			return
		}
		var data map[string]any
		if err := json.Unmarshal(body, &data); err != nil {
			fmt.Printf("FAIL %s: bad JSON — %s\n", name, string(body))
			fail++
			return
		}
		if check != nil && !check(data) {
			fmt.Printf("FAIL %s: check failed — %s\n", name, truncate(string(body), 200))
			fail++
			return
		}
		fmt.Printf("PASS %s\n", name)
		pass++
	}

	// Health — should be sqlite_offline mode
	test("Health", "/health", func(d map[string]any) bool {
		return d["mode"] == "sqlite_offline"
	})

	// Smart Search — OEM number
	test("SmartSearch OEM 26300-35505", "/api/search?q="+url.QueryEscape("26300-35505"), func(d map[string]any) bool {
		total, _ := d["total"].(float64)
		strategy, _ := d["searchStrategy"].(string)
		return total > 0 && strategy != ""
	})

	// Smart Search — text
	test("SmartSearch text 'oil filter'", "/api/search?q="+url.QueryEscape("oil filter"), func(d map[string]any) bool {
		total, _ := d["total"].(float64)
		return total > 0
	})

	// Smart Search — part number
	test("SmartSearch article 'OC 205'", "/api/search?q="+url.QueryEscape("OC 205"), func(d map[string]any) bool {
		total, _ := d["total"].(float64)
		return total > 0
	})

	// OEM lookup
	test("OEM Lookup 26300-35505", "/api/oem/"+url.PathEscape("26300-35505"), func(d map[string]any) bool {
		total, _ := d["total"].(float64)
		return total > 0
	})

	// Catalog models
	test("Catalog Models HYUNDAI", "/api/catalog/models?make=HYUNDAI", func(d map[string]any) bool {
		models, ok := d["models"].([]any)
		return ok && len(models) > 0
	})

	// Catalog vehicles
	test("Catalog Vehicles HYUNDAI TUCSON", "/api/catalog/vehicles?make=HYUNDAI&model=TUCSON", func(d map[string]any) bool {
		vehicles, ok := d["vehicles"].([]any)
		return ok && len(vehicles) > 0
	})

	// Catalog assembly groups
	test("Catalog Groups VID=10001", "/api/catalog/groups?vehicleId=10001", func(d map[string]any) bool {
		groups, ok := d["groups"].([]any)
		return ok && len(groups) > 0
	})

	// Catalog parts
	test("Catalog Parts VID=10001 Group=10100", "/api/catalog/parts?vehicleId=10001&groupId=10100", func(d map[string]any) bool {
		parts, ok := d["parts"].([]any)
		return ok && len(parts) > 0
	})

	// Vehicle parts
	test("Vehicle Parts VID=10001", "/api/vehicle/10001/parts", func(d map[string]any) bool {
		total, _ := d["total"].(float64)
		return total > 0
	})

	// Vehicle categories
	test("Vehicle Categories VID=10001", "/api/vehicle/10001/categories", func(d map[string]any) bool {
		cats, ok := d["categories"].([]any)
		return ok && len(cats) > 0
	})

	// Cross-ref for article
	test("CrossRef article 100001", "/api/part/100001/crossref", func(d map[string]any) bool {
		oems, ok := d["oemNumbers"].([]any)
		return ok && len(oems) > 0
	})

	// Smart Search — cabin filter with vehicle
	test("SmartSearch vehicle+text", "/api/search?q="+url.QueryEscape("cabin filter")+"&linkageTargetId=10001", func(d map[string]any) bool {
		total, _ := d["total"].(float64)
		return total > 0
	})

	// KIA models
	test("Catalog Models KIA", "/api/catalog/models?make=KIA", func(d map[string]any) bool {
		models, ok := d["models"].([]any)
		return ok && len(models) > 0
	})

	// KIA Sportage vehicle + parts
	test("Catalog Vehicles KIA SPORTAGE", "/api/catalog/vehicles?make=KIA&model=SPORTAGE", func(d map[string]any) bool {
		vehicles, ok := d["vehicles"].([]any)
		return ok && len(vehicles) > 0
	})

	// False positive — bogus OEM
	test("Reject bogus OEM", "/api/search?q="+url.QueryEscape("99999-ZZ999"), func(d map[string]any) bool {
		total, _ := d["total"].(float64)
		warnings, _ := d["warnings"].([]any)
		return total == 0 && len(warnings) > 0
	})

	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("Results: %d PASS, %d FAIL out of %d tests\n", pass, fail, pass+fail)
	if fail > 0 {
		os.Exit(1)
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
