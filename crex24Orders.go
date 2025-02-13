package main

import (
	"backpocket/models"
	"backpocket/utils"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

/*
	Crex24 Order Status:
		unfilledActive - converted to NEW
		filled - converted to FILLED,
		unfilledCancelled -  converted to CANCELLED
		partiallyFilledCancelled - converted to CANCELLED
		partiallyFilledActive - converted to PARTIALLY_FILLED
*/

var (
	chanRestartCrex24OrderStream = make(chan bool, 10)
)

type crex24OrderType struct {
	ID uint64

	Instrument, Side, Type,
	Status, TimeInForce,
	Timestamp, Lastupdate string

	Volume, Price, RemainingVolume float64
}

func crex24OrderStream() {
	// for {
	//loop through enabled markets.

	//get the crex24Orders linked to the order

	// 	time.After(time.Second * 15)
	// }
}

func crex24AllOrders(pair string) {

	queryParams := fmt.Sprintf(crex24ListOrdersParams, pair)
	respBytes := crex24RestAPI("GET", "/v2/trading/orderHistory?"+queryParams, nil)

	//Check if Response is an Error
	crex24CheckError(respBytes)

	var crex24OrderList []crex24OrderType
	json.Unmarshal(respBytes, &crex24OrderList)
	for _, crex24Order := range crex24OrderList {
		crex24UpdateOrder(crex24Order)
	}
}

func crex24OrderQuery(orderid uint64) {

	queryParams := fmt.Sprintf(crex24OrderQueryParams, orderid)
	respBytes := crex24RestAPI("GET", "/v2/trading/orderStatus?"+queryParams, nil)

	//Check if Response is an Error
	crex24CheckError(respBytes)

	crex24Order := crex24OrderType{}
	json.Unmarshal(respBytes, &crex24Order)

	newOrder := getOrder(crex24Order.ID, "crex24")

	switch crex24Order.Status {
	default:
		newOrder.Status = "NEW"
	case "unfilledActive":
		newOrder.Status = "NEW"
	case "filled":
		newOrder.Status = "FILLED"
	case "unfilledCancelled":
		newOrder.Status = "CANCELLED"
	case "partiallyFilledCancelled":
		newOrder.Status = "CANCELLED"
	case "partiallyFilledActive":
		newOrder.Status = "PARTIALLY_FILLED"
	}

	newOrder.Status = crex24Order.Status
	newOrder.Updatedate, _ = time.Parse(utils.TimeFormat, crex24Order.Lastupdate)
	updateOrderAndSave(newOrder, true)

	crex24CheckError(respBytes)

}

func crex24OrderCreate(pair, side string, price, quantity, stoploss, takeprofit float64, autorepeat int, reforderid uint64) {

	queryParams := fmt.Sprintf(crex24OrderCreateParams, pair, side, price, quantity)
	respBytes := crex24RestAPI("POST", "/v2/trading/placeOrder", []byte(queryParams))

	//Check if Response is an Error
	crex24CheckError(respBytes)

	crex24Order := crex24OrderType{}
	json.Unmarshal(respBytes, &crex24Order)

	//--> New Order being created -
	if crex24Order.ID > 0 {
		order := models.Order{}
		order.Exchange = "crex24"
		order.Stoploss = stoploss
		order.Takeprofit = takeprofit
		order.AutoRepeat = autorepeat
		order.RefOrderID = uint64(reforderid)

		order.Side = crex24Order.Side
		order.OrderID = crex24Order.ID
		order.Pair = crex24Order.Instrument
		order.Status = crex24Order.Status
		order.Createdate, _ = time.Parse(utils.TimeFormat, crex24Order.Timestamp)

		order.Price = crex24Order.Price
		order.Quantity = crex24Order.Volume

		order.Total = order.Price * order.Quantity

		if err := utils.SqlDB.Model(&order).Create(&order).Error; err != nil {
			log.Println(err.Error())
		}

		wsBroadcastNotification <- notifications{
			Title:   "*Crex24 Exchange*",
			Message: fmt.Sprintf("%s limit %s order [%v] for %v %s", order.Status, order.Side, order.OrderID, order.Quantity, order.Pair),
		}
	}
	//--> New Order being created -

	prvOrder := getOrder(uint64(reforderid), "crex24")
	newOrder := getOrder(crex24Order.ID, "crex24")

	prvOrder.RefOrderID = crex24Order.ID

	newOrder.Stoploss = stoploss
	newOrder.Takeprofit = takeprofit

	if newOrder.Stoploss > 0 || newOrder.Takeprofit > 0 {
		newOrder.RefEnabled = 1
	}

	newOrder.RefOrderID = uint64(reforderid)

	updateOrderAndSave(prvOrder, true)

	updateOrderAndSave(newOrder, true)
}

func crex24OrderCancel(orderid uint64) {
	orderParams := fmt.Sprintf(crex24OrderCancelParams, orderid)
	respBytes := crex24RestAPI("POST", "/v2/trading/cancelOrdersById", []byte(orderParams))

	//Check if Response is an Error
	crex24CheckError(respBytes)

	crex24Order := crex24OrderType{}
	json.Unmarshal(respBytes, &crex24Order)

	cancelledOrder := getOrder(crex24Order.ID, "crex24")
	cancelledOrder.Status = crex24Order.Status
	cancelledOrder.Updatedate, _ = time.Parse(utils.TimeFormat, crex24Order.Lastupdate)
	updateOrderAndSave(cancelledOrder, true)
}

func crex24UpdateOrder(crex24Order crex24OrderType) {

	if crex24Order.Instrument == "" {
		return
	}

	order := getOrder(crex24Order.ID, "crex24")
	// sqlCheck := "select * from orders where orderid = $1 and exchange = 'crex24'"
	// utils.SqlDB.Get(&order, sqlCheck, crex24Order.ID)

	order.Exchange = "crex24"
	if !(order.ID > 0) {
		order.Side = crex24Order.Side
		order.Pair = crex24Order.Instrument
		order.OrderID = crex24Order.ID
		order.Status = crex24Order.Status
		order.Createdate, _ = time.Parse(utils.TimeFormat, crex24Order.Timestamp)

		order.Price = crex24Order.Price
		order.Quantity = crex24Order.Volume
		order.Total = order.Price * order.Quantity

		if err := utils.SqlDB.Model(&order).Create(&order).Error; err != nil {
			log.Println(err.Error())
		}

	} else {
		order.Status = crex24Order.Status
		order.Updatedate, _ = time.Parse(utils.TimeFormat, crex24Order.Lastupdate)
	}

	updateOrderAndSave(order, true)
}
