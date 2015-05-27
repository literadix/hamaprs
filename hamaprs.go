package hamaprs

// #cgo LDFLAGS: -lfap
/*
#include <fap.h>
#include <stdlib.h>

// type is a reserved keyword in Go, we need something to reach p->type
fap_packet_type_t getPacketType(fap_packet_t* p) {
	if (!p) return -1;
    if (p->type != NULL) return *p->type;
    return -1;
}
*/
import "C"
import (
	"errors"
	"runtime"
	"strings"
	"time"
	"unsafe"
)

type PacketType int

const (
	LocationPacketType PacketType = iota
	ObjectPacketType
	ItemPacketType
	MicePacketType
	NMEAPacketType
	WXPacketType
	MessagePacketType
	CapabilitiesPacketType
	StatusPacketType
	TelemetryPacketType
	TelemetryMessagePacketType
	DXSpotPacketType
	ExperimentalPacketType
	InvalidPacketType
)

const InvalidCoordinate float64 = 360

// Packet describes an APRS packet
type Packet struct {
	PacketType
	Timestamp           int
	SourceCallsign      string
	DestinationCallsign string
	Status              string
	Symbol              string
	Latitude            float64
	Longitude           float64
	Altitude            float64
	Speed               float64
	Course              uint8
	Weather             *WeatherReport
	RawMessage          string
	MicE                string
	Message             string
	Comment             string
}

// WeatherReport describes the weather related part of an APRS packet
type WeatherReport struct {
	Temperature       float64
	InsideTemperature float64
	Humidity          uint8
	InsideHumidity    uint8
	WindGust          float64
	WindDirection     uint8
	WindSpeed         float64
	Pressure          float64
}

// Telemetry describes the telemetry related part of an APRS packet
type Telemetry struct {
	Val1, Val2, Val3, Val4, Val5 float64
}

// Parser is an APRS Parser
type Parser struct{}

// Returns a new APRS Parser
func NewParser() *Parser {
	C.fap_init()
	p := &Parser{}
	runtime.SetFinalizer(p, func() {
		C.fap_cleanup()
	})
	return p
}

// ParsePacket parse raw packet string and return a new Packet
func (p *Parser) ParsePacket(raw string, isAX25 bool) (*Packet, error) {
	packet := &Packet{Latitude: InvalidCoordinate, Longitude: InvalidCoordinate}
	return p.FillAprsPacket(raw, isAX25, packet)
}

func (p *Parser) FillAprsPacket(raw string, isAX25 bool, packet *Packet) (*Packet, error) {
	message_cstring := C.CString(raw)
	message_length := C.uint(len(raw))
	defer C.free(unsafe.Pointer(message_cstring))

	cpacket := C.fap_parseaprs(message_cstring, message_length, C.short(boolToInt(isAX25)))

	defer C.fap_free(cpacket)

	if cpacket.error_code != nil {
		return nil, errors.New("Unable to parse APRS message")
	}

	packet.Timestamp = int(time.Now().Unix())
	packet.SourceCallsign = strings.ToUpper(C.GoString(cpacket.src_callsign))
	packet.DestinationCallsign = strings.ToUpper(C.GoString(cpacket.dst_callsign))
	packet.Latitude = parseNilableCoordinate(cpacket.latitude)
	packet.Longitude = parseNilableCoordinate(cpacket.longitude)
	packet.Speed = parseNilableFloat(cpacket.speed)
	packet.Course = parseNilableUInt(cpacket.course)
	packet.Altitude = parseNilableFloat(cpacket.altitude)
	packet.Message = C.GoString(cpacket.message)
	packet.Status = C.GoStringN(cpacket.status, C.int(cpacket.status_len))
	packet.Comment = C.GoStringN(cpacket.comment, C.int(cpacket.comment_len))
	packet.RawMessage = raw

	switch C.getPacketType(cpacket) {
	case C.fapLOCATION:
		packet.PacketType = LocationPacketType
	case C.fapOBJECT:
		packet.PacketType = ObjectPacketType
	case C.fapITEM:
		packet.PacketType = ItemPacketType
	case C.fapMICE:
		packet.PacketType = MicePacketType
	case C.fapNMEA:
		packet.PacketType = NMEAPacketType
	case C.fapWX:
		packet.PacketType = WXPacketType
	case C.fapMESSAGE:
		packet.PacketType = MessagePacketType
	case C.fapCAPABILITIES:
		packet.PacketType = CapabilitiesPacketType
	case C.fapSTATUS:
		packet.PacketType = StatusPacketType
	case C.fapTELEMETRY:
		packet.PacketType = TelemetryPacketType
	case C.fapTELEMETRY_MESSAGE:
		packet.PacketType = TelemetryMessagePacketType
	case C.fapDX_SPOT:
		packet.PacketType = DXSpotPacketType
	case C.fapEXPERIMENTAL:
		packet.PacketType = ExperimentalPacketType
	default:
		packet.PacketType = InvalidPacketType
	}

	if cpacket.wx_report != nil {
		w := WeatherReport{
			Temperature:       parseNilableFloat(cpacket.wx_report.temp),
			InsideTemperature: parseNilableFloat(cpacket.wx_report.temp_in),
			Humidity:          parseNilableUInt(cpacket.wx_report.humidity),
			InsideHumidity:    parseNilableUInt(cpacket.wx_report.humidity_in),
			WindGust:          parseNilableFloat(cpacket.wx_report.wind_gust),
			WindDirection:     parseNilableUInt(cpacket.wx_report.wind_dir),
			WindSpeed:         parseNilableFloat(cpacket.wx_report.wind_speed),
			Pressure:          parseNilableFloat(cpacket.wx_report.pressure),
		}
		packet.Weather = &w
	}

	// MicE alloc a buffer of 20 bytes for fap_mice_mbits_to_message C func
	cbuffer := (*C.char)(C.malloc(C.size_t(20)))
	defer C.free(unsafe.Pointer(cbuffer))

	if cpacket.messagebits != nil {
		C.fap_mice_mbits_to_message(cpacket.messagebits, cbuffer)
		packet.MicE = C.GoString(cbuffer)
	}

	return packet, nil
}

// IncludePosition return true if the packet contains a Position
func (p *Packet) IncludePosition() bool {
	if p.Latitude != InvalidCoordinate && p.Longitude != InvalidCoordinate {
		return true
	}
	return false
}

// return a short version of the callsign as KK6NXK for KK6NXK-7
func ShortCallsign(c string) string {
	s := strings.Split(c, "-")
	return s[0]
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func parseNilableFloat(d *C.double) float64 {
	if d != nil {
		return float64(C.double(*d))
	}
	return 0
}

func parseNilableCoordinate(d *C.double) float64 {
	if d != nil {
		return float64(C.double(*d))
	}
	return InvalidCoordinate
}

func parseNilableUInt(d *C.uint) uint8 {
	if d != nil {
		return uint8(C.uint(*d))
	}
	return 0
}