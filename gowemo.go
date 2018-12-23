package main

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
)

const (
	destinationAddr = "239.255.255.250"
	destPort        = "1900"
	msearch         = "M-SEARCH * HTTP/1.1\r\n" + "HOST: 239.255.255.250:1900\r\n" + "MAN: \"ssdp:discover\"\r\n" + "MX: 5\r\n" + "ST: ssdp:all\r\n\r\n"
	datagramSize    = 4096
)

func splitAndDisplay(message string) {
	messageSegments := strings.Split(message, "\r\n")

	for i, segment := range messageSegments {
		fmt.Printf("%2d: %s\r\n", i, segment)
	}
}

func findLocation(message string) string {
	const locationField = "LOCATION: "
	messageSegments := strings.Split(message, "\r\n")
	location := "No location"

	for _, segment := range messageSegments {
		if strings.HasPrefix(segment, locationField) {
			locationSegment := strings.Split(segment, locationField)

			location = strings.TrimSpace(locationSegment[1])
			break
		}
	}

	return location
}

func udpDiscovery(serv *net.UDPConn, verbose bool) string {
	// Read responses.
	pServ := make([]byte, datagramSize) // receving buffer.
	serv.SetReadBuffer(datagramSize)
	for {
		_, remoteAddr, readErr := serv.ReadFromUDP(pServ)

		if readErr != nil {
			fmt.Printf("*** Error reading from UDP: %v\n\n\n", readErr)
			continue
		}

		response := string(pServ)
		if strings.Contains(response, "Belkin") && strings.Contains(response, "setup.xml") {
			if verbose {
				fmt.Printf("[ --- Got response from %v --- ]\n", remoteAddr)
				fmt.Printf("      Location: %s\n\n\n", findLocation(response))
				fmt.Printf("      PAYLOAD\n\n")
				splitAndDisplay(response)
				fmt.Printf("\n\n\n")
			}

			return strings.TrimSuffix(findLocation(response), "/setup.xml")
		}

		if verbose {
			fmt.Printf("[ --- Ignoring response from %v --- ]\n", remoteAddr)
		}
	}
}

func doHTTPRequest(addr string, method string, soapAction string, xmlPayload string) string {
	request, _ := http.NewRequest(method, addr, strings.NewReader(xmlPayload))
	request.Header = map[string][]string{
		"Content-Type": {"text/xml; charset=\"utf-8\""},
		"SOAPACTION":   {soapAction},
	}

	client := &http.Client{}
	response, respErr := client.Do(request)

	if respErr != nil {
		fmt.Printf("Response error [%v]", respErr)
	}

	pResp := make([]byte, datagramSize)
	response.Body.Read(pResp)

	return string(pResp)
}

func doGetBinaryState(deviceAddr string) bool {
	getBinaryStateXML := "<?xml " +
		"version=\"1.0\" " +
		"encoding=\"utf-8\" " +
		"?>" +
		"<s:Envelope " +
		"xmlns:s=\"http://schemas.xmlsoap.org/soap/envelope/\" " +
		"s:encodingStyle=\"http://schemas.xmlsoap.org/soap/encoding/\"> " +
		"<s:Body> " +
		"<u:GetBinaryState " +
		"xmlns:u=\"urn:Belkin:service:basicevent:1\"> " +
		"</u:GetBinaryState> " +
		"</s:Body> " +
		"</s:Envelope>"

	response := doHTTPRequest(deviceAddr+"/upnp/control/basicevent1", "POST", "\"urn:Belkin:service:basicevent:1#GetBinaryState\"", getBinaryStateXML)
	return parseGetBinaryState(response)
}

func parseGetBinaryState(message string) bool {
	const field = "<BinaryState>"
	messageSegments := strings.Split(message, "\r\n")
	currState := false

	for _, segment := range messageSegments {
		if strings.HasPrefix(segment, field) {
			if segment[len(field)] == '1' {
				currState = true
			}
		}
	}

	return currState
}

func doSetBinaryState(deviceAddr string, newState bool) string {
	var state = "0"
	if newState {
		state = "1"
	}

	setBinaryStateXML := "<?xml " +
		"version=\"1.0\" " +
		"encoding=\"utf-8\" " +
		"?>" +
		"<s:Envelope " +
		"xmlns:s=\"http://schemas.xmlsoap.org/soap/envelope/\" " +
		"s:encodingStyle=\"http://schemas.xmlsoap.org/soap/encoding/\"> " +
		"<s:Body> " +
		"<u:SetBinaryState " +
		"xmlns:u=\"urn:Belkin:service:basicevent:1\"> " +
		"<BinaryState>" + state +
		"</BinaryState> " +
		"</u:SetBinaryState> " +
		"</s:Body> " +
		"</s:Envelope>"

	return doHTTPRequest(deviceAddr+"/upnp/control/basicevent1", "POST", "\"urn:Belkin:service:basicevent:1#SetBinaryState\"", setBinaryStateXML)
}

func main() {
	fmt.Print("Wemo Test\n*********\n\n\n")

	client, connErr := net.Dial("udp", destinationAddr+":"+destPort)
	if connErr != nil {
		fmt.Printf("*** Error binding UDP socket %v\n\n\n", connErr)
	}

	fmt.Printf("[Searching via UDP]:\n%s", msearch)
	fmt.Fprintf(client, msearch)

	// Fetch bound address
	servAddr, _ := net.ResolveUDPAddr("udp", client.LocalAddr().String())

	// Be a good neighbour, close your sockets.
	client.Close()

	// setup receiving server code.
	serv, servErr := net.ListenUDP("udp", servAddr)
	if servErr != nil {
		fmt.Printf("Error listening: %v\n", servErr)
	}

	deviceLocation := udpDiscovery(serv, false)
	currentState := doGetBinaryState(deviceLocation)
	fmt.Println("Found Belkin device at addr: " + deviceLocation)
	fmt.Printf("Currently ON/OFF state [%t]\n\n\n", currentState)

	fmt.Println("Hit <ENTER> to switch state")
	bufio.NewReader(os.Stdin).ReadBytes('\n')

	fmt.Printf("Setting ON/OFF state to [%t]\n", !currentState)
	doSetBinaryState(deviceLocation, !currentState)

	currentState = doGetBinaryState(deviceLocation)
	fmt.Printf("New ON/OFF state [%t]\n", currentState)

	bufio.NewReader(os.Stdin).ReadBytes('\n')
	fmt.Println("Quitting...")
}
