package router

import (
	"sync"
	"testing"
	"time"

	pb_broker "github.com/TheThingsNetwork/ttn/api/broker"
	pb_gateway "github.com/TheThingsNetwork/ttn/api/gateway"
	pb_protocol "github.com/TheThingsNetwork/ttn/api/protocol"
	pb_lorawan "github.com/TheThingsNetwork/ttn/api/protocol/lorawan"
	pb "github.com/TheThingsNetwork/ttn/api/router"
	"github.com/TheThingsNetwork/ttn/core"
	"github.com/TheThingsNetwork/ttn/core/router/gateway"
	"github.com/TheThingsNetwork/ttn/core/types"
	. "github.com/TheThingsNetwork/ttn/utils/testing"
	. "github.com/smartystreets/assertions"
)

// newReferenceDownlink returns a default uplink message
func newReferenceDownlink() *pb.DownlinkMessage {
	up := &pb.DownlinkMessage{
		Payload: make([]byte, 20),
		ProtocolConfiguration: &pb_protocol.TxConfiguration{Protocol: &pb_protocol.TxConfiguration_Lorawan{Lorawan: &pb_lorawan.TxConfiguration{
			CodingRate: "4/5",
			DataRate:   "SF7BW125",
			Modulation: pb_lorawan.Modulation_LORA,
		}}},
		GatewayConfiguration: &pb_gateway.TxConfiguration{
			Timestamp: 100,
			Frequency: 868100000,
		},
	}
	return up
}

func TestHandleDownlink(t *testing.T) {
	a := New(t)

	r := &router{
		Component: &core.Component{
			Ctx: GetLogger(t, "TestHandleActivation"),
		},
		gateways:        map[types.GatewayEUI]*gateway.Gateway{},
		brokerDiscovery: &mockBrokerDiscovery{},
	}

	eui := types.GatewayEUI{0, 1, 2, 3, 4, 5, 6, 7}
	id, _ := r.getGateway(eui).Schedule.GetOption(0, 10*1000)
	err := r.HandleDownlink(&pb_broker.DownlinkMessage{
		Payload: []byte{},
		DownlinkOption: &pb_broker.DownlinkOption{
			GatewayEui:     &eui,
			Identifier:     id,
			ProtocolConfig: &pb_protocol.TxConfiguration{},
			GatewayConfig:  &pb_gateway.TxConfiguration{},
		},
	})

	a.So(err, ShouldBeNil)
}

func TestSubscribeUnsubscribeDownlink(t *testing.T) {
	a := New(t)

	r := &router{
		Component: &core.Component{
			Ctx: GetLogger(t, "TestHandleActivation"),
		},
		gateways:        map[types.GatewayEUI]*gateway.Gateway{},
		brokerDiscovery: &mockBrokerDiscovery{},
	}

	eui := types.GatewayEUI{0, 1, 2, 3, 4, 5, 6, 7}
	ch, err := r.SubscribeDownlink(eui)
	a.So(err, ShouldBeNil)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		var gotDownlink bool
		for dl := range ch {
			gotDownlink = true
			a.So(dl.Payload, ShouldResemble, []byte{0x02})
		}
		a.So(gotDownlink, ShouldBeTrue)
		wg.Done()
	}()

	id, _ := r.getGateway(eui).Schedule.GetOption(0, 10*1000)
	r.HandleDownlink(&pb_broker.DownlinkMessage{
		Payload: []byte{0x02},
		DownlinkOption: &pb_broker.DownlinkOption{
			GatewayEui:     &eui,
			Identifier:     id,
			ProtocolConfig: &pb_protocol.TxConfiguration{},
			GatewayConfig:  &pb_gateway.TxConfiguration{},
		},
	})

	// Wait for the downlink to arrive
	<-time.After(5 * time.Millisecond)

	err = r.UnsubscribeDownlink(eui)
	a.So(err, ShouldBeNil)

	wg.Wait()
}

func TestUplinkBuildDownlinkOptions(t *testing.T) {
	a := New(t)

	r := &router{}

	// If something is incorrect, it just returns an empty list
	up := &pb.UplinkMessage{}
	gtw := gateway.NewGateway(types.GatewayEUI{0, 1, 2, 3, 4, 5, 6, 7})
	options := r.buildDownlinkOptions(up, false, gtw)
	a.So(options, ShouldBeEmpty)

	// The reference gateway and uplink work as expected
	gtw, up = newReferenceGateway("EU_863_870"), newReferenceUplink()
	options = r.buildDownlinkOptions(up, false, gtw)
	a.So(options, ShouldHaveLength, 2)
	a.So(options[0].Score, ShouldBeLessThan, options[1].Score)

	// Check Delay
	a.So(options[0].GatewayConfig.Timestamp, ShouldEqual, 1000100)
	a.So(options[1].GatewayConfig.Timestamp, ShouldEqual, 2000100)

	// Check Frequency
	a.So(options[0].GatewayConfig.Frequency, ShouldEqual, 868100000)
	a.So(options[1].GatewayConfig.Frequency, ShouldEqual, 869525000)

	// Check Power
	a.So(options[0].GatewayConfig.Power, ShouldEqual, 14)
	a.So(options[1].GatewayConfig.Power, ShouldEqual, 27)

	// Check Data Rate
	a.So(options[0].ProtocolConfig.GetLorawan().DataRate, ShouldEqual, "SF7BW125")
	a.So(options[1].ProtocolConfig.GetLorawan().DataRate, ShouldEqual, "SF9BW125")

	// Check Coding Rate
	a.So(options[0].ProtocolConfig.GetLorawan().CodingRate, ShouldEqual, "4/5")
	a.So(options[1].ProtocolConfig.GetLorawan().CodingRate, ShouldEqual, "4/5")

	// And for joins we want a different delay (both RX1 and RX2) and DataRate (RX2)
	gtw, up = newReferenceGateway("EU_863_870"), newReferenceUplink()
	options = r.buildDownlinkOptions(up, true, gtw)
	a.So(options[0].GatewayConfig.Timestamp, ShouldEqual, 5000100)
	a.So(options[1].GatewayConfig.Timestamp, ShouldEqual, 6000100)
	a.So(options[1].ProtocolConfig.GetLorawan().DataRate, ShouldEqual, "SF12BW125")
}

func TestUplinkBuildDownlinkOptionsFrequencies(t *testing.T) {
	a := New(t)

	r := &router{}

	// Unsupported frequencies use only RX2 for downlink
	gtw, up := newReferenceGateway("EU_863_870"), newReferenceUplink()
	up.GatewayMetadata.Frequency = 869300000
	options := r.buildDownlinkOptions(up, false, gtw)
	a.So(options, ShouldHaveLength, 1)

	// Supported frequencies use RX1 (on the same frequency) for downlink
	ttnEUFrequencies := []uint64{
		868100000,
		868300000,
		868500000,
		867100000,
		867300000,
		867500000,
		867700000,
		867900000,
	}
	for _, freq := range ttnEUFrequencies {
		up = newReferenceUplink()
		up.GatewayMetadata.Frequency = freq
		options := r.buildDownlinkOptions(up, false, gtw)
		a.So(options, ShouldHaveLength, 2)
		a.So(options[0].GatewayConfig.Frequency, ShouldEqual, freq)
	}

	// Unsupported frequencies use only RX2 for downlink
	gtw, up = newReferenceGateway("US_902_928"), newReferenceUplink()
	up.GatewayMetadata.Frequency = 923300000
	options = r.buildDownlinkOptions(up, false, gtw)
	a.So(options, ShouldHaveLength, 1)

	// Supported frequencies use RX1 (on the same frequency) for downlink
	ttnUSFrequencies := map[uint64]uint64{
		903900000: 923300000,
		904100000: 923900000,
		904300000: 924500000,
		904500000: 925100000,
		904700000: 925700000,
		904900000: 926300000,
		905100000: 926900000,
		905300000: 927500000,
	}
	for upFreq, downFreq := range ttnUSFrequencies {
		up = newReferenceUplink()
		up.GatewayMetadata.Frequency = upFreq
		options := r.buildDownlinkOptions(up, false, gtw)
		a.So(options, ShouldHaveLength, 2)
		a.So(options[0].GatewayConfig.Frequency, ShouldEqual, downFreq)
	}

	// Unsupported frequencies use only RX2 for downlink
	gtw, up = newReferenceGateway("AU_915_928"), newReferenceUplink()
	up.GatewayMetadata.Frequency = 923300000
	options = r.buildDownlinkOptions(up, false, gtw)
	a.So(options, ShouldHaveLength, 1)

	// Supported frequencies use RX1 (on the same frequency) for downlink
	ttnAUFrequencies := map[uint64]uint64{
		916800000: 923300000,
		917000000: 923900000,
		917200000: 924500000,
		917400000: 925100000,
		917600000: 925700000,
		917800000: 926300000,
		918000000: 926900000,
		918200000: 927500000,
	}
	for upFreq, downFreq := range ttnAUFrequencies {
		up = newReferenceUplink()
		up.GatewayMetadata.Frequency = upFreq
		options := r.buildDownlinkOptions(up, false, gtw)
		a.So(options, ShouldHaveLength, 2)
		a.So(options[0].GatewayConfig.Frequency, ShouldEqual, downFreq)
	}
}

func TestUplinkBuildDownlinkOptionsDataRate(t *testing.T) {
	a := New(t)

	r := &router{}

	gtw := newReferenceGateway("EU_863_870")

	// Supported datarates use RX1 (on the same datarate) for downlink
	ttnEUDataRates := []string{
		"SF7BW125",
		"SF8BW125",
		"SF9BW125",
		"SF10BW125",
		"SF11BW125",
		"SF12BW125",
	}
	for _, dr := range ttnEUDataRates {
		up := newReferenceUplink()
		up.ProtocolMetadata.GetLorawan().DataRate = dr
		options := r.buildDownlinkOptions(up, false, gtw)
		a.So(options, ShouldHaveLength, 2)
		a.So(options[0].ProtocolConfig.GetLorawan().DataRate, ShouldEqual, dr)
	}

	gtw = newReferenceGateway("US_902_928")

	// Test 500kHz channel
	up := newReferenceUplink()
	up.GatewayMetadata.Frequency = 904600000
	up.ProtocolMetadata.GetLorawan().DataRate = "SF8BW500"
	options := r.buildDownlinkOptions(up, false, gtw)
	a.So(options, ShouldHaveLength, 2)
	a.So(options[0].ProtocolConfig.GetLorawan().DataRate, ShouldEqual, "SF7BW500")

	// Supported datarates use RX1 (on the same datarate) for downlink
	ttnUSDataRates := map[string]string{
		"SF7BW125":  "SF7BW500",
		"SF8BW125":  "SF8BW500",
		"SF9BW125":  "SF9BW500",
		"SF10BW125": "SF10BW500",
	}
	for drUp, drDown := range ttnUSDataRates {
		up := newReferenceUplink()
		up.GatewayMetadata.Frequency = 903900000
		up.ProtocolMetadata.GetLorawan().DataRate = drUp
		options := r.buildDownlinkOptions(up, false, gtw)
		a.So(options, ShouldHaveLength, 2)
		a.So(options[0].ProtocolConfig.GetLorawan().DataRate, ShouldEqual, drDown)
	}

	gtw = newReferenceGateway("AU_915_928")

	// Test 500kHz channel
	up = newReferenceUplink()
	up.GatewayMetadata.Frequency = 917500000
	up.ProtocolMetadata.GetLorawan().DataRate = "SF8BW500"
	options = r.buildDownlinkOptions(up, false, gtw)
	a.So(options, ShouldHaveLength, 2)
	a.So(options[0].ProtocolConfig.GetLorawan().DataRate, ShouldEqual, "SF7BW500")

	// Supported datarates use RX1 (on the same datarate) for downlink
	ttnAUDataRates := map[string]string{
		"SF7BW125":  "SF7BW500",
		"SF8BW125":  "SF8BW500",
		"SF9BW125":  "SF9BW500",
		"SF10BW125": "SF10BW500",
	}
	for drUp, drDown := range ttnAUDataRates {
		up := newReferenceUplink()
		up.GatewayMetadata.Frequency = 916800000
		up.ProtocolMetadata.GetLorawan().DataRate = drUp
		options := r.buildDownlinkOptions(up, false, gtw)
		a.So(options, ShouldHaveLength, 2)
		a.So(options[0].ProtocolConfig.GetLorawan().DataRate, ShouldEqual, drDown)
	}
}

// Note: This test uses r.buildDownlinkOptions which in turn calls computeDownlinkScores
func TestComputeDownlinkScores(t *testing.T) {
	a := New(t)
	r := &router{}
	gtw := newReferenceGateway("EU_863_870")
	refScore := r.buildDownlinkOptions(newReferenceUplink(), false, gtw)[0].Score

	// Lower RSSI -> worse score
	testSubject := newReferenceUplink()
	testSubject.GatewayMetadata.Rssi = -80.0
	testSubjectgtw := newReferenceGateway("EU_863_870")
	testSubjectScore := r.buildDownlinkOptions(testSubject, false, testSubjectgtw)[0].Score
	a.So(testSubjectScore, ShouldBeGreaterThan, refScore)

	// Lower SNR -> worse score
	testSubject = newReferenceUplink()
	testSubject.GatewayMetadata.Snr = 2.0
	testSubjectgtw = newReferenceGateway("EU_863_870")
	testSubjectScore = r.buildDownlinkOptions(testSubject, false, testSubjectgtw)[0].Score
	a.So(testSubjectScore, ShouldBeGreaterThan, refScore)

	// Slower DataRate -> worse score
	testSubject = newReferenceUplink()
	testSubject.ProtocolMetadata.GetLorawan().DataRate = "SF10BW125"
	testSubjectgtw = newReferenceGateway("EU_863_870")
	testSubjectScore = r.buildDownlinkOptions(testSubject, false, testSubjectgtw)[0].Score
	a.So(testSubjectScore, ShouldBeGreaterThan, refScore)

	// Gateway used for Rx -> worse score
	testSubject1 := newReferenceUplink()
	testSubject2 := newReferenceUplink()
	testSubject2.GatewayMetadata.Timestamp = 10000000
	testSubject2.GatewayMetadata.Frequency = 868500000
	testSubjectgtw = newReferenceGateway("EU_863_870")
	testSubjectgtw.Utilization.AddRx(newReferenceUplink())
	testSubjectgtw.Utilization.Tick()
	testSubject1Score := r.buildDownlinkOptions(testSubject1, false, testSubjectgtw)[0].Score
	testSubject2Score := r.buildDownlinkOptions(testSubject2, false, testSubjectgtw)[0].Score
	a.So(testSubject1Score, ShouldBeGreaterThan, refScore)          // Because of Rx in the gateway
	a.So(testSubject2Score, ShouldBeGreaterThan, refScore)          // Because of Rx in the gateway
	a.So(testSubject1Score, ShouldBeGreaterThan, testSubject2Score) // Because of Rx on the same channel

	// European Alarm Band
	// NOTE: This frequency is not part of the TTN DownlinkChannels. This test
	// case makes sure we don't allow Tx on the alarm bands even if someone
	// changes the frequency plan.
	testSubject = newReferenceUplink()
	testSubject.GatewayMetadata.Frequency = 869300000
	testSubjectgtw = newReferenceGateway("EU_863_870")
	options := r.buildDownlinkOptions(testSubject, false, testSubjectgtw)
	a.So(options, ShouldHaveLength, 1) // RX1 Removed
	a.So(options[0].GatewayConfig.Frequency, ShouldNotEqual, 869300000)

	// European Duty-cycle
	testSubject = newReferenceUplink()
	testSubjectgtw = newReferenceGateway("EU_863_870")
	for i := 0; i < 5; i++ {
		testSubjectgtw.Utilization.AddTx(newReferenceDownlink())
	}
	testSubjectgtw.Utilization.Tick()
	options = r.buildDownlinkOptions(testSubject, false, testSubjectgtw)
	a.So(options, ShouldHaveLength, 1) // RX1 Removed
	a.So(options[0].GatewayConfig.Frequency, ShouldNotEqual, 868100000)

	// Scheduling Conflicts
	testSubject1 = newReferenceUplink()
	testSubject2 = newReferenceUplink()
	testSubject2.GatewayMetadata.Timestamp = 2000000
	testSubjectgtw = newReferenceGateway("EU_863_870")
	testSubjectgtw.Schedule.GetOption(1000100, 50000)
	testSubject1Score = r.buildDownlinkOptions(testSubject1, false, testSubjectgtw)[0].Score
	testSubject2Score = r.buildDownlinkOptions(testSubject2, false, testSubjectgtw)[0].Score
	a.So(testSubject1Score, ShouldBeGreaterThan, refScore) // Scheduling conflict with RX1
	a.So(testSubject2Score, ShouldEqual, refScore)         // No scheduling conflicts
}
