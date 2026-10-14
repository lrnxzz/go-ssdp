package ssdp

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

const udaAlive = "NOTIFY * HTTP/1.1\r\n" +
	"HOST: 239.255.255.250:1900\r\n" +
	"CACHE-CONTROL: max-age=1800\r\n" +
	"LOCATION: http://192.0.2.20:49152/description.xml\r\n" +
	"NT: urn:schemas-upnp-org:device:MediaRenderer:1\r\n" +
	"NTS: ssdp:alive\r\n" +
	"SERVER: unix/5.1 UPnP/1.1 MyProduct/1.0\r\n" +
	"USN: uuid:2fac1234-31f8-11b4-a222-08002b34c003::urn:schemas-upnp-org:device:MediaRenderer:1\r\n" +
	"BOOTID.UPNP.ORG: 1\r\n" +
	"CONFIGID.UPNP.ORG: 7\r\n" +
	"SEARCHPORT.UPNP.ORG: 49153\r\n" +
	"\r\n"

const udaByeBye = "NOTIFY * HTTP/1.1\r\n" +
	"HOST: 239.255.255.250:1900\r\n" +
	"NT: urn:schemas-upnp-org:device:MediaRenderer:1\r\n" +
	"NTS: ssdp:byebye\r\n" +
	"USN: uuid:2fac1234-31f8-11b4-a222-08002b34c003::urn:schemas-upnp-org:device:MediaRenderer:1\r\n" +
	"BOOTID.UPNP.ORG: 1\r\n" +
	"CONFIGID.UPNP.ORG: 7\r\n" +
	"\r\n"

const udaUpdate = "NOTIFY * HTTP/1.1\r\n" +
	"HOST: 239.255.255.250:1900\r\n" +
	"LOCATION: http://192.0.2.20:49152/description.xml\r\n" +
	"NT: urn:schemas-upnp-org:device:MediaRenderer:1\r\n" +
	"NTS: ssdp:update\r\n" +
	"USN: uuid:2fac1234-31f8-11b4-a222-08002b34c003::urn:schemas-upnp-org:device:MediaRenderer:1\r\n" +
	"BOOTID.UPNP.ORG: 1\r\n" +
	"CONFIGID.UPNP.ORG: 7\r\n" +
	"NEXTBOOTID.UPNP.ORG: 2\r\n" +
	"SEARCHPORT.UPNP.ORG: 49153\r\n" +
	"\r\n"

func Test_Notifications_MarshalBinary_WriteTheUdaLayout(t *testing.T) {
	tests := map[string]Message{
		udaAlive:  exampleAlive(t),
		udaByeBye: exampleByeBye(t),
		udaUpdate: exampleUpdate(t),
	}

	for expected, notification := range tests {
		if got := string(marshal(t, notification)); got != expected {
			t.Errorf("Expected\n%q\ngot\n%q", expected, got)
		}
	}
}

func Test_Notifications_UnmarshalBinary_ReadEveryField(t *testing.T) {
	var (
		alive  Alive
		byeBye ByeBye
		update Update
	)

	if err := alive.UnmarshalBinary([]byte(udaAlive)); err != nil || !reflect.DeepEqual(alive, exampleAlive(t)) {
		t.Errorf("Expected the example alive, got %+v, %v", alive, err)
	}

	if err := byeBye.UnmarshalBinary([]byte(udaByeBye)); err != nil || !reflect.DeepEqual(byeBye, exampleByeBye(t)) {
		t.Errorf("Expected the example byebye, got %+v, %v", byeBye, err)
	}

	if err := update.UnmarshalBinary([]byte(udaUpdate)); err != nil || !reflect.DeepEqual(update, exampleUpdate(t)) {
		t.Errorf("Expected the example update, got %+v, %v", update, err)
	}

	expectConformant(t, alive)
	expectConformant(t, byeBye)
	expectConformant(t, update)
}

func Test_Alive_Check_FollowsTheDeclaredVersion(t *testing.T) {
	alive := exampleAlive(t)
	alive.BootId = Optional[int]{}
	alive.ConfigId = Optional[int]{}

	expectViolations(t, alive, 2)

	alive.Server = mustProductTokens(t, "unix/5.1 UPnP/1.0 MyProduct/1.0")
	expectConformant(t, alive)
}

func Test_Alive_Check_ReportsEveryMissingField(t *testing.T) {
	alive := Alive{
		Target:      newRootDeviceTarget(),
		ServiceName: newServiceName(exampleUuid, newRootDeviceTarget()),
	}

	expectViolations(t, alive, 4)
}

func Test_Notifications_Check_RejectSsdpAllAsNt(t *testing.T) {
	alive := exampleAlive(t)
	alive.Target = newAllTarget()

	byeBye := exampleByeBye(t)
	byeBye.Target = newAllTarget()

	update := exampleUpdate(t)
	update.Target = newAllTarget()

	notifications := []Message{
		alive,
		byeBye,
		update,
	}

	for _, notification := range notifications {
		expectViolations(t, notification, 1)
	}
}

func Test_ByeBye_Check_RequiresHost(t *testing.T) {
	byeBye := exampleByeBye(t)
	byeBye.Host = ""

	expectViolations(t, byeBye, 1)
}

func Test_Update_Check_RequiresNextBootIdAboveBootId(t *testing.T) {
	nextBootIds := []int{
		1,
		0,
	}

	for _, nextBootId := range nextBootIds {
		update := exampleUpdate(t)
		update.NextBootId = NewOptional(nextBootId)

		expectViolations(t, update, 1)
	}
}

func Test_Update_Check_RequiresItsCounters(t *testing.T) {
	update := exampleUpdate(t)
	update.BootId = Optional[int]{}
	update.ConfigId = Optional[int]{}
	update.NextBootId = Optional[int]{}

	expectViolations(t, update, 3)
}

func Test_Notifications_MarshalBinary_RefuseMissingIdentity(t *testing.T) {
	notifications := []Message{
		Alive{
			Target: newRootDeviceTarget(),
		},
		ByeBye{
			ServiceName: newServiceName(exampleUuid, newRootDeviceTarget()),
		},
		Update{},
	}

	for _, notification := range notifications {
		if _, err := notification.MarshalBinary(); !errors.Is(err, ErrMalformed) {
			t.Errorf("Expected a %s without NT or USN to be refused, got %v", notification.Type(), err)
		}
	}
}

func Test_Notifications_UnmarshalBinary_RejectBrokenFields(t *testing.T) {
	packets := []string{
		strings.Replace(udaUpdate, "NEXTBOOTID.UPNP.ORG: 2", "NEXTBOOTID.UPNP.ORG: two", 1),
		strings.Replace(udaAlive, "SEARCHPORT.UPNP.ORG: 49153", "SEARCHPORT.UPNP.ORG: 1900", 1),
		strings.Replace(udaAlive, "CONFIGID.UPNP.ORG: 7", "CONFIGID.UPNP.ORG: -7", 1),
		strings.Replace(udaAlive, "NT: "+exampleDevice+"\r\n", "", 1),
		strings.Replace(udaByeBye, "USN: uuid:", "USN: ", 1),
	}

	for _, packet := range packets {
		if _, err := Decode([]byte(packet)); !errors.Is(err, ErrMalformed) {
			t.Errorf("Expected %q to be malformed, got %v", packet, err)
		}
	}
}

func Test_Notifications_Type(t *testing.T) {
	tests := map[MessageType]Message{
		MessageAlive:  Alive{},
		MessageByeBye: ByeBye{},
		MessageUpdate: Update{},
	}

	for expected, message := range tests {
		if got := message.Type(); got != expected {
			t.Errorf("Expected %s, got %s", expected, got)
		}
	}
}
