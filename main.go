package main

import (
	"fmt"
	"image/color"
	"machine"
	"math/rand"
	"time"

	"tinygo.org/x/drivers/net/mqtt"
	"tinygo.org/x/drivers/sh1107"
	"tinygo.org/x/drivers/trellis"
	"tinygo.org/x/drivers/wifinina"
	"tinygo.org/x/tinyfont"

	//"tinygo.org/x/tinyfont/freemono"
	"tinygo.org/x/tinyfont/proggy"
)

var NINA_SPI = machine.SPI0

// NINA-m4 express pins
const (
	NINA_SDO    machine.Pin = machine.PB23
	NINA_SDI    machine.Pin = machine.PB22
	NINA_CS     machine.Pin = machine.PA23
	NINA_SCK    machine.Pin = machine.PA17
	NINA_GPIO0  machine.Pin = machine.PA20
	NINA_RESETN machine.Pin = machine.PA22
	NINA_ACK    machine.Pin = machine.PA21
	NINA_TX     machine.Pin = machine.PB16
	NINA_RX     machine.Pin = machine.PB17
)

var (
	bat     = machine.ADC{machine.PB01}
	spi     = NINA_SPI
	adaptor *wifinina.Device
	cl      mqtt.Client
	topic   = "trellis"
	//font    = &freemono.Bold9pt7b
	//font = &freemono.Regular9pt7b
	//font = &freemono.Regular8pt7b
	font    = &proggy.TinySZ8pt7b
	white   = color.RGBA{255, 255, 255, 255}
	display sh1107.Device
)

func main() {
	fmt.Println("Start")
	time.Sleep(5 * time.Second)
	err := machine.I2C0.Configure(machine.I2CConfig{
		// I think these are the defaults
		Frequency: machine.TWI_FREQ_400KHZ,
		SCL:       machine.SCL_PIN,
		SDA:       machine.SDA_PIN,
	})
	//err := machine.I2C0.Configure(machine.I2CConfig{})
	if err != nil {
		println("could not configure I2C:", err)
		return
	}

	display = sh1107.New(machine.I2C0, 0x3C, 64, 128, false)
	fmt.Println("Configure Display")
	display.Configure()
	display.ClearDisplay()

	// this interrupt pin is not being used
	in := machine.D5
	in.Configure(machine.PinConfig{Mode: machine.PinInput})
	in.High()

	tr := trellis.New(machine.I2C0, 0x70, 10)
	fmt.Println("New New")
	tr.Configure()
	fmt.Println("Configure")
	time.Sleep(1 * time.Second)

	err = machine.SPI0.Configure(machine.SPIConfig{Frequency: 2000000})
	if err != nil {
		println(err)
	}

	spi.Configure(machine.SPIConfig{
		Frequency: 8 * 1e6,
		SDO:       NINA_SDO, //MOSI = machine.SPIO_SDO_PIN
		SDI:       NINA_SDI, //MISO = machine.SPIO_SDI_PIN
		SCK:       NINA_SCK, //SCK = machine.SPIO_SCK_PIN
	})

	time.Sleep(5 * time.Second)

	// Init wifit
	adaptor = wifinina.New(spi,
		NINA_CS,
		NINA_ACK,
		NINA_GPIO0,
		NINA_RESETN,
	)
	//adaptor.Configure()
	adaptor.Configure2(false)   //true = reset active high
	time.Sleep(5 * time.Second) // necessary
	fw, err := adaptor.GetFwVersion()
	if err != nil {
		println("GetFwVersion Error:", err)
	}
	//println("firmware:", fw)
	//tinyfont.WriteLineRotated(&display, font, 1, 10, fmt.Sprintf("fmwr: %s", fw), white, tinyfont.NO_ROTATION)
	tinyfont.WriteLineRotated(&display, font, 50, 0, fmt.Sprintf("fmwr: %s", fw), white, tinyfont.ROTATION_90)
	display.Display()

	for {
		err := connectToAP()
		if err == nil {
			break
		}
	}

	opts := mqtt.NewClientOptions()
	clientID := "tinygo-client-" + randomString(5)
	opts.AddBroker(server).SetClientID(clientID)
	//println(clientID)
	//tinyfont.WriteLineRotated(&display, font, 1, 30, fmt.Sprintf("id: %s", clientID), white, tinyfont.NO_ROTATION)
	tinyfont.WriteLineRotated(&display, font, 40, 0, fmt.Sprintf("id: %s", clientID), white, tinyfont.ROTATION_90)
	display.Display()
	//opts.AddBroker(server).SetClientID("tinygo-client-2")

	println("Connecting to MQTT broker at", server)
	cl = mqtt.NewClient(opts)
	token := cl.Connect()

	if token.Wait() && token.Error() != nil {
		failMessage("mqtt connect", token.Error().Error())
	}

	var i uint8
	for {
		trellis.Clear()
		time.Sleep(1 * time.Millisecond)
		trellis.SetLED(i)
		time.Sleep(1 * time.Millisecond)
		tr.WriteDisplay()
		i += 1
		if i == 16 {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	t := time.Now()
	for {
		b := tr.ReadSwitches()
		//Need some delay here 20 ms works for i2c 400MHZ but YMMV
		time.Sleep(20 * time.Millisecond)

		var k uint8
		if b {
			for k = 0; k < 16; k++ {
				if trellis.IsKeyPressed(k) {
					//if trellis.JustPressed(k) {
					sendMessage(k)
					trellis.Clear()
					trellis.SetLED(k)
					time.Sleep(1 * time.Millisecond)
					tr.WriteDisplay()
					time.Sleep(1 * time.Second) // simple debouncing
					break
				}
			}
		}
		if time.Since(t) > time.Minute {
			token := cl.Pingreq()
			if token.Error() != nil {
				failMessage("ping", token.Error().Error())
			}
			t = time.Now()
		}
		time.Sleep(1 * time.Millisecond)
	}
}

func connectToAP() error {
	time.Sleep(2 * time.Second)
	println("Connecting to " + ssid)
	err := adaptor.ConnectToAccessPoint(ssid, pass, 10*time.Second)
	if err != nil {
		println(err)
		return err
	}

	println("Connected.")

	time.Sleep(2 * time.Second)
	ip, _, _, err := adaptor.GetIP()
	for ; err != nil; ip, _, _, err = adaptor.GetIP() {
		println(err.Error())
		time.Sleep(1 * time.Second)
	}
	println(ip.String())
	return nil
}

// Returns an int >= min, < max
func randomInt(min, max int) int {
	return min + rand.Intn(max-min)
}

// Generate a random string of A-Z chars with len = l
func randomString(len int) string {
	bytes := make([]byte, len)
	for i := 0; i < len; i++ {
		bytes[i] = byte(randomInt(65, 90))
	}
	return string(bytes)
}

func failMessage(action, msg string) {
	println(action, ": ", msg)
	time.Sleep(5 * time.Second)
}

func sendMessage(key uint8) {
	println("Publishing MQTT message...")
	msg := fmt.Sprintf(`{"key":%d}`, key)

	//data := []byte(fmt.Sprintf(`{"key":%d}`, key))
	token := cl.Publish(topic, 0, false, []byte(msg))
	token.Wait()
	if err := token.Error(); err != nil {
		switch t := err.(type) {
		case wifinina.Error:
			println(t.Error(), "attempting to reconnect")
			if token := cl.Connect(); token.Wait() && token.Error() != nil {
				failMessage("mqtt send", token.Error().Error())
			}
		default:
			println(err.Error())
		}
	} else {
		display.ClearDisplay()
		//tinyfont.WriteLineRotated(&display, font, 1, 10, msg, white, tinyfont.NO_ROTATION)
		tinyfont.WriteLineRotated(&display, font, 1, 10, msg, white, tinyfont.ROTATION_90)
		display.Display()
	}
}
