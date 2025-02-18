package main

import (
	"backpocket/models"
	"backpocket/utils"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

var (
	// "1m":  []string{"1m", "5m", "15m"},
	// "5m":  []string{"5m", "15m", "30m"},
	// "15m": []string{"15m", "30m", "1h"},
	// "30m": []string{"30m", "1h", "4h"},
	// "1h":  []string{"1h", "4h", "6h"},
	// "4h":  []string{"4h", "6h", "12h"},
	// "6h":  []string{"6h", "12h", "1d"},
	// "12h": []string{"12h", "1d", "3d"},

	TimeframeMaps = map[string][]string{
		"1d":  []string{"1d", "1M"},
		"12h": []string{"12h", "1w"},
		"6h":  []string{"6h", "3d"},
		"4h":  []string{"4h", "1d"},
		// "2h":  []string{"2h", "1d"},
		"1h":  []string{"1h", "12h"},
		"30m": []string{"30m", "6h"},
		"15m": []string{"15m", "4h"},
		"5m":  []string{"5m", "1h"},
		"3m":  []string{"3m", "30m"},
		"1m":  []string{"1m", "15m"},
	}
)

func restHandlerOpportunity(httpRes http.ResponseWriter, httpReq *http.Request) {
	query := httpReq.URL.Query()

	pair := query.Get("pair")
	exchange := query.Get("exchange")
	timeframe := query.Get("intervals")
	limit := query.Get("limit")
	startTime := query.Get("starttime")
	endTime := query.Get("endtime")
	marketPriceVar := query.Get("marketprice")

	marketPrice, err := strconv.ParseFloat(marketPriceVar, 64)
	if err != nil {
		marketPrice = 0
	}

	if exchange == "" {
		exchange = "binance"
	}

	if pair == "" {
		http.Error(httpRes, "Missing pair parameter", http.StatusBadRequest)
		return
	}

	if len(TimeframeMaps[timeframe]) != 2 {
		timeframe = "1m"
	}

	intervals := strings.Join(TimeframeMaps[timeframe], ",") + ",1m"
	analysis, err := retrieveMarketPairAnalysis(pair, exchange, limit, endTime, startTime, intervals)
	if err != nil {
		http.Error(httpRes, err.Error(), http.StatusInternalServerError)
		return
	}
	opportunity := analyseOpportunity(analysis, timeframe, marketPrice)

	httpRes.Header().Set("Content-Type", "application/json")
	jsonResponse, err := json.Marshal(opportunity)
	if err != nil {
		http.Error(httpRes, "Error converting to JSON", http.StatusInternalServerError)
		return
	}

	httpRes.Write(jsonResponse)
}
func restHandlerSearchOpportunity(httpRes http.ResponseWriter, httpReq *http.Request) {
	query := httpReq.URL.Query()

	pair := query.Get("pair")
	action := query.Get("action")
	exchange := query.Get("exchange")
	timeframe := query.Get("timeframe")

	starttime := query.Get("starttime")
	endtime := query.Get("endtime")

	var searchText string
	var searchParams []interface{}

	if pair != "" {
		searchText = " pair like ? "
		searchParams = append(searchParams, pair)
	}

	if action != "" {
		if searchText != "" {
			searchText += " AND "
		}
		searchText = " action like ? "
		searchParams = append(searchParams, action)
	}

	if exchange != "" {
		if searchText != "" {
			searchText += " AND "
		}
		searchText = " exchange like ? "
		searchParams = append(searchParams, exchange)
	}

	if timeframe != "" {
		if searchText != "" {
			searchText += " AND "
		}
		searchText = " timeframe like ? "
		searchParams = append(searchParams, timeframe)
	}

	if starttime != "" {
		if searchText != "" {
			searchText += " AND "
		}
		searchText += " createdate >= ?::timestamp "
		searchParams = append(searchParams, starttime)
	}

	if endtime != "" {
		if searchText != "" {
			searchText += " AND "
		}
		searchText += " createdate <= ?::timestamp "
		searchParams = append(searchParams, endtime)
	}

	orderby := "createdate desc"

	var filteredOrderList []models.Opportunity
	if err := utils.SqlDB.Where(searchText, searchParams...).Order(orderby).Find(&filteredOrderList).Error; err != nil {
		http.Error(httpRes, err.Error(), http.StatusInternalServerError)
	}

	httpRes.Header().Set("Content-Type", "application/json")
	jsonResponse, err := json.Marshal(filteredOrderList)
	if err != nil {
		http.Error(httpRes, "Error converting to JSON", http.StatusInternalServerError)
		return
	}

	httpRes.Write(jsonResponse)
}

type opportunityType struct {
	Pair       string
	Action     string
	Price      float64
	Timeframe  string
	Exchange   string
	Stoploss   float64
	Takeprofit float64
	Analysis   map[string]interface{}
}

func analyseOpportunity(analysis analysisType, timeframe string, price float64) (opportunity opportunityType) {
	if analysis.Pair == "" || analysis.Exchange == "" {
		return
	}

	if len(TimeframeMaps[timeframe]) != 2 {
		return
	}

	market := getMarket(analysis.Pair, analysis.Exchange)
	if price == 0 {
		price = market.Price
	}

	for _, interval := range analysis.Intervals {
		interval.Candle.Close = price
		interval.Trend = utils.OverallTrend(interval.SMA10.Entry,
			interval.SMA20.Entry, interval.SMA50.Entry, interval.Candle.Close)
	}

	// log.Printf("\n\n---1m Candle----: %+v", analysis.Intervals["1m"].Pattern)

	interval1m := analysis.Intervals["1m"]
	lowerInterval := analysis.Intervals[TimeframeMaps[timeframe][0]]
	upperInterval := analysis.Intervals[TimeframeMaps[timeframe][1]]

	if price == 0 {
		price = interval1m.Candle.Close
	}
	opportunity.Pair = analysis.Pair
	opportunity.Exchange = analysis.Exchange
	opportunity.Timeframe = timeframe
	opportunity.Price = price

	isCheckLong := checkIfLong(price, lowerInterval, upperInterval)

	//Check for Long // Buy Opportunity
	if isCheckLong {
		opportunity.Action = "BUY"
	}

	// -- -- --

	//Check for Short // Sell Opportunity
	isCheckShort := checkIfShort(price, lowerInterval, upperInterval)
	if isCheckShort {
		opportunity.Action = "SELL"
	}

	switch opportunity.Action {
	case "BUY":
		opportunity.Stoploss = upperInterval.SMA20.Support
		opportunity.Takeprofit = lowerInterval.SMA50.Resistance
	case "SELL":
		opportunity.Stoploss = upperInterval.SMA20.Resistance
		opportunity.Takeprofit = lowerInterval.SMA50.Support
	}

	// opportunity.Analysis = map[string]interface{}{
	// 	"Buy":  buyAnalysis,
	// 	"Sell": sellAnalysis,
	// }

	if market.Closed == 1 {
		opportunityMutex.Lock()
		pairexchange := fmt.Sprintf("%s-%s", analysis.Pair, analysis.Exchange)
		opportunityMap[pairexchange] = notifications{Title: "", Message: ""}
		opportunityMutex.Unlock()
	}

	return
}

func checkIfLong(currentPrice float64, summaryLower, summaryUpper utils.Summary) bool {
	checkLong := map[string]bool{
		"rsi":       true,
		"fib":       true,
		"trend":     true,
		"bollinger": true,
		"support":   true,
	}

	if summaryLower.RSI == 0 || summaryUpper.RSI == 0 {
		return false
	}

	if checkLong["fib"] {
		checkLong["fib"] = currentPrice < summaryUpper.RetracementLevels["0.786"]
	}
	if checkLong["bollinger"] {
		checkLong["bollinger"] = currentPrice < summaryUpper.BollingerBands["middle"]
	}
	if checkLong["trend"] {
		checkLong["trend"] = summaryUpper.Trend != "Bullish"
	}

	if checkLong["fib"] {
		checkLong["fib"] = currentPrice < summaryLower.RetracementLevels["0.786"]
	}
	if checkLong["bollinger"] {
		checkLong["bollinger"] = currentPrice < summaryLower.BollingerBands["lower"]
	}
	if checkLong["trend"] {
		checkLong["trend"] = summaryLower.Trend == "Bearish"
	}
	if checkLong["rsi"] {
		checkLong["rsi"] = summaryLower.RSI < 50
	}
	if checkLong["support"] {
		checkLong["support"] = summaryLower.SMA10.Support == summaryLower.SMA50.Support
	}

	return checkLong["fib"] && checkLong["bollinger"] && checkLong["trend"] && checkLong["rsi"] && checkLong["support"]
}

func checkIfShort(currentPrice float64, summaryLower, summaryUpper utils.Summary) bool {
	checkShort := map[string]bool{
		"rsi":        true,
		"fib":        true,
		"trend":      true,
		"bollinger":  true,
		"resistance": true,
	}

	if summaryLower.RSI == 0 || summaryUpper.RSI == 0 {
		return false
	}

	if checkShort["fib"] {
		checkShort["fib"] = currentPrice > summaryUpper.RetracementLevels["0.236"]
	}
	if checkShort["bollinger"] {
		checkShort["bollinger"] = currentPrice > summaryUpper.BollingerBands["middle"]
	}
	if checkShort["trend"] {
		checkShort["trend"] = summaryUpper.Trend != "Bearish"
	}

	if checkShort["fib"] {
		checkShort["fib"] = currentPrice > summaryLower.RetracementLevels["0.236"]
	}
	if checkShort["bollinger"] {
		checkShort["bollinger"] = currentPrice > summaryLower.BollingerBands["upper"]
	}
	if checkShort["trend"] {
		checkShort["trend"] = summaryLower.Trend == "Bullish"
	}
	if checkShort["rsi"] {
		checkShort["rsi"] = summaryLower.RSI > 50
	}
	if checkShort["resistance"] {
		checkShort["resistance"] = summaryLower.SMA10.Resistance == summaryLower.SMA50.Resistance
	}

	return checkShort["fib"] && checkShort["bollinger"] && checkShort["trend"] && checkShort["rsi"] && checkShort["resistance"]
}
