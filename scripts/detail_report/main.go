package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const baseURL = "http://localhost:8080"

type SmartSearchResponse struct {
	Query          string        `json:"query"`
	Results        []SmartResult `json:"results"`
	Total          int           `json:"total"`
	SearchStrategy string        `json:"searchStrategy"`
}

type SmartResult struct {
	LegacyArticleId         int                `json:"legacyArticleId"`
	ArticleNumber           string             `json:"articleNumber"`
	Description             string             `json:"description"`
	BrandName               string             `json:"brandName"`
	Substitutions           []SubstitutionPart `json:"substitutions"`
	AftermarketAlternatives []AftermarketPart  `json:"aftermarketAlternatives"`
	Compatibility           []string           `json:"compatibility"`
}

type AftermarketPart struct {
	PartNumber  string `json:"partNumber"`
	Description string `json:"description"`
	Brand       string `json:"brand"`
}

type SubstitutionPart struct {
	PartNumber  string `json:"partNumber"`
	Description string `json:"description"`
	Make        string `json:"make"`
}

func main() {
	if !waitForServer(3) {
		fmt.Println("FATAL: Server not reachable at", baseURL)
		os.Exit(1)
	}

	parts := []string{
		"26300-35505",
		"97133-D3000",
		"28113-D3100",
		"27301-2B100",
		"25310-2S500",
		"35310-2S000",
		"21810-2S000",
		"39210-2B100",
		"54651-D3000",
		"97701-D3000",
		"52933-1P000",
		"58101-D3A70",
		"98350-D3100",
		"92101-D3100",
		"56820-D3000",
		"86350-D3100",
		"OC 205",
		"W 811/80",
	}

	fmt.Println("╔══════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    PARTS ENGINE — DETAILED REPORT                           ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	for i, pn := range parts {
		printPartReport(i+1, pn)
	}

	fmt.Println("══════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("Total parts queried: %d\n", len(parts))
}

func printPartReport(idx int, partNum string) {
	resp, err := http.Get(baseURL + "/api/search?q=" + url.QueryEscape(partNum) + "&limit=20")
	if err != nil {
		fmt.Printf("  [%02d] %s — ERROR: %v\n\n", idx, partNum, err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var r SmartSearchResponse
	json.Unmarshal(body, &r)

	fmt.Printf("──────────────────────────────────────────────────────────────────────────────\n")
	fmt.Printf("  [%02d] Query: %-20s  Strategy: %-25s  Results: %d\n", idx, partNum, r.SearchStrategy, r.Total)
	fmt.Printf("──────────────────────────────────────────────────────────────────────────────\n")

	if r.Total == 0 {
		fmt.Printf("       (no results)\n\n")
		return
	}

	for ri, res := range r.Results {
		fmt.Printf("\n  Result #%d:\n", ri+1)
		fmt.Printf("    Part Number : %s\n", res.ArticleNumber)
		fmt.Printf("    Description : %s\n", res.Description)
		fmt.Printf("    Brand       : %s\n", res.BrandName)

		if len(res.Substitutions) > 0 {
			fmt.Printf("    Substitutions (%d):\n", len(res.Substitutions))
			for _, sub := range res.Substitutions {
				make_ := sub.Make
				if make_ == "" {
					make_ = "-"
				}
				desc := sub.Description
				if desc == "" {
					desc = "-"
				}
				fmt.Printf("      • %-18s  %-35s  [%s]\n", sub.PartNumber, desc, make_)
			}
		}

		if len(res.AftermarketAlternatives) > 0 {
			fmt.Printf("    Aftermarket Alternatives (%d):\n", len(res.AftermarketAlternatives))
			for _, alt := range res.AftermarketAlternatives {
				desc := alt.Description
				if desc == "" {
					desc = "-"
				}
				fmt.Printf("      • %-18s  %-35s  [%s]\n", alt.PartNumber, desc, alt.Brand)
			}
		} else {
			fmt.Printf("    Aftermarket Alternatives: (none)\n")
		}

		if len(res.Compatibility) > 0 {
			compStr := strings.Join(res.Compatibility, ", ")
			if len(compStr) > 80 {
				compStr = compStr[:77] + "..."
			}
			fmt.Printf("    Compatibility (%d): %s\n", len(res.Compatibility), compStr)
		}
	}
	fmt.Println()
}

func waitForServer(seconds int) bool {
	for i := 0; i < seconds; i++ {
		resp, err := http.Get(baseURL + "/health")
		if err == nil {
			resp.Body.Close()
			return true
		}
		time.Sleep(time.Second)
	}
	return false
}
