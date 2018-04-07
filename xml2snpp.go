package main

import (
	"log"
	"net"

	"github.com/ClinicalSystemsEngineering/snpp"
	"gopkg.in/natefinch/lumberjack.v2" //rotational logging
	//"strings"
	"encoding/xml"
	"flag"
	//"os"
	"html/template"
	"net/http"
	"strconv"
)

//Page is the r5 xml structure.  although an r5 message contains Type it has been omitted for now.
type Page struct {
	ID      string `xml:"ID"`
	TagText string `xml:"TagText"`
	//Type string `xml:"Type"`
}

type webpage struct {
	Title   string
	Heading string
	Body    []string
	Nav     []string
}

//used in the nav element to easily have a bottom navigation area on each page
var webpageurls = []string{"home", "status", "page"}

var queuesize = 0 //the size of the processed message channel

var parsedmsgs = make(chan string, 10000) //message processing channel for xml2snpp conversions

//HomePage definitions
func HomePage(w http.ResponseWriter, req *http.Request) {

	homepage := webpage{Title: "XML2SNPP Homepage", Heading: "List of Commands:", Nav: webpageurls}

	tpl, err := template.ParseFiles("index.gohtml")
	if err != nil {
		log.Printf("error parsing index template: %v", err)
	}
	err = tpl.ExecuteTemplate(w, "index.gohtml", homepage)
	if err != nil {
		log.Printf("error executing template index: %v", err)
	}
}

//StatusPage displays the size of the queue for monitoring purposes
//queue values above 100 are considered an error if the queue stays at 100 or above for a prolonged period
//this obviously can be adjusted below to suit.
func StatusPage(w http.ResponseWriter, req *http.Request) {
	//get the latest queue value
	queuemonitor()

	var queuestatus []string

	//determine if queue size is in error state or not currently hardcoded to 100
	if queuesize <= 100 {
		queuestatus = append(queuestatus, "OK: Current Queue Size:"+strconv.Itoa(queuesize))
	} else {
		queuestatus = append(queuestatus, "ERROR: Current Queue Size:"+strconv.Itoa(queuesize))
	}

	statuspage := webpage{Title: "XML2SNPP Statuspage", Heading: "Current Queue Status:", Body: queuestatus, Nav: webpageurls}

	tpl, err := template.ParseFiles("status.gohtml")
	if err != nil {
		log.Printf("error parsing status template: %v", err)
	}
	err = tpl.ExecuteTemplate(w, "status.gohtml", statuspage)
	if err != nil {
		log.Printf("error executing status template: %v", err)
	}

}

//SendPage request page for pin;message to add to the queue for processing
func SendPage(w http.ResponseWriter, req *http.Request) {

	if req.Method == "GET" {
		sendpage := webpage{Title: "XML2SNPP Sendpage", Heading: "Input pin and message below:", Nav: webpageurls}

		tpl, err := template.ParseFiles("sendpage.gohtml")
		if err != nil {
			log.Printf("error parsing sendpage template: %v", err)
		}
		err = tpl.ExecuteTemplate(w, "sendpage.gohtml", sendpage)
		if err != nil {
			log.Printf("error executing send template: %v", err)
		}
	} else {
		// put pin and message into the processing queue
		req.ParseForm()
		pin := req.PostFormValue("pin")
		msg := req.PostFormValue("message")
		if pin != "" && msg != "" {
			parsedmsgs <- pin + ";" + msg
			sendpage := webpage{Title: "XML2SNPP Sendpage", Heading: "Message submitted. Input pin and message below:", Nav: webpageurls}

			tpl, err := template.ParseFiles("sendpage.gohtml")
			if err != nil {
				log.Printf("error parsing sendpage template: %v", err)
			}
			err = tpl.ExecuteTemplate(w, "sendpage.gohtml", sendpage)
			if err != nil {
				log.Printf("error executing send template: %v", err)
			}
		} else {
			sendpage := webpage{Title: "XML2SNPP Sendpage", Heading: "Error with Submission Input Try Again. Input pin and message below:", Nav: webpageurls}

			tpl, err := template.ParseFiles("sendpage.gohtml")
			if err != nil {
				log.Printf("error parsing sendpage template: %v", err)
			}
			err = tpl.ExecuteTemplate(w, "sendpage.gohtml", sendpage)
			if err != nil {
				log.Printf("error executing send template: %v", err)
			}
		}

	}

}

func webserver(portnum string) {
	http.HandleFunc("/", HomePage)
	http.HandleFunc("/home", HomePage)
	http.HandleFunc("/status", StatusPage)
	http.HandleFunc("/page", SendPage)
	http.Handle("/favicon.ico", http.NotFoundHandler())
	for {
		log.Println(http.ListenAndServe(":"+portnum, nil))
	}
}

func queuemonitor() {

	queuesize = len(parsedmsgs)

}

//example r5 xml
//<Page xmlns:xsi='http://www.w3.org/2001/XMLSchema-instance' xmlns:xsd='http://www.w3.org/2001/XMLSchema'>
//	<ID>89699</ID>
//	<TagText>4906 beeping</TagText>
//   <Type>Phone/Pager</Type>
//</Page>

//example response for a ___PING___
//<?xml version="1.0" encoding="utf-8"?> <PageTXSrvResp State="7" PagesInQueue="4" PageOK="1" />

//main can accept 3 flag arguments the port for the xml listener and the port
//for the SNPP output and the port for the http server
//i.e call xml2snpp -xmlPort=5051 -snppCon:hostname:port -httpPort=80
//default ports are 5051 for xml , 444 for snpp and 80 for http
func main() {

	xmlPort := flag.String("xmlPort", "5051", "xml listener port for localhost")
	httpPort := flag.String("httpPort", "80", "localhost listner port for http server")
	snppConn := flag.String("snppCon", "127.0.0.1:444", "snpp server address and port in the form serverip:port")
	flag.Parse()

	log.SetOutput(&lumberjack.Logger{
		Filename:   "/var/log/xml2snpp/xml2snpp.log",
		MaxSize:    100, // megabytes
		MaxBackups: 5,
		MaxAge:     60,   //days
		Compress:   true, // disabled by default
	})

	log.Printf("STARTING XML Listener on tcp port %v\n\n", *xmlPort)
	l, err := net.Listen("tcp", ":"+*xmlPort)
	if err != nil {
		log.Println("Error opening XML listener, check log for details")
		log.Fatal(err)
	}
	defer l.Close()

	//start snpp client
	go snpp.Client(parsedmsgs, *snppConn)

	//start a webserver
	go webserver(*httpPort)

	for {

		// Listen for an incoming xml connection.
		conn, err := l.Accept()
		if err != nil {
			log.Println("Error accepting: ", err.Error())
			log.Fatal(err)
		}

		// Handle connections in a new goroutine.
		go func(c net.Conn, msgs chan<- string) {
			//set up a decoder on the stream
			d := xml.NewDecoder(c)

			for {
				// Look for the next token
				// Note that this only reads until the next positively identified
				// XML token in the stream
				t, err := d.Token()
				if err != nil {
					log.Printf("Token error %v\n", err.Error())
					break
				}
				switch et := t.(type) {

				case xml.StartElement:
					// search for Page start element and decode
					if et.Name.Local == "Page" {
						p := &Page{}
						// decode the page element while automagically advancing the stream
						// if no matching token is found, there will be an error
						// note the search only happens within the parent.
						if err := d.DecodeElement(&p, &et); err != nil {
							log.Printf("error decoding element %v\n", err.Error())
							c.Close()
							return
						}

						// We have decoded the xml message now send it off to TAP server or reply if ping
						log.Printf("Parsed: Pin:%v;Msg:%v\n", p.ID, p.TagText)

						//note the R5 system periodically sends out a PING looking for a response
						//this will handle that response or put the decoded xml into the TAP output queue
						if p.ID == "" && p.TagText == "___PING___" {
							//send response to connection
							response := "<?xml version=\"1.0\" encoding=\"utf-8\"?> <PageTXSrvResp State=\"7\" PagesInQueue=\"0\" PageOK=\"1\" />"
							log.Printf("Responding:%v\n", response)
							c.Write([]byte(response))
						} else {
							parsedmsgs <- string(p.ID) + ";" + string(p.TagText)

						}

					}

				case xml.EndElement:
					if et.Name.Local != "Page" {
						continue
					}
				}

			}

			c.Close()
		}(conn, parsedmsgs)
	}

}
