package tm1637

import (
	"errors"
	"fmt"
	"sync"
	"time"
	"unicode"

	"github.com/stianeikeland/go-rpio/v4"
)

// TM1637 commands
const (
	cmdDataAutoAddr    byte = 0x40 // Data command: write data to display register, auto-increment address
	cmdDataFixedAddr   byte = 0x44 // Data command: write data to display register, fixed address (not used in this simplified version)
	cmdDisplayCtrlBase byte = 0x80 // Base for display control command (brightness, on/off)
	cmdAddrBase        byte = 0xC0 // Base for address setting command (grid 0)

	MaxBrightness     byte = 7
	DefaultBrightness byte = 2 // A moderate brightness level
	NumDigits         int  = 4
)


// TM1637 represents a TM1637 7-segment display driver using go-rpio.
type TM1637 struct {
	clkPin rpio.Pin
	dioPin rpio.Pin

	brightness   byte
	colonEnabled bool

	mu    sync.Mutex
	delay time.Duration // Communication delay, typically a few microseconds

	digitToSegment map[rune]byte
}

// defaultSegmentMap provides a basic mapping from characters to 7-segment display codes.
// Bit order: DP.G.F.E.D.C.B.A (MSB to LSB: bit7=DP, bit6=G, ..., bit0=A)
var defaultSegmentMap = map[rune]byte{
	'0': 0x3f, '1': 0x06, '2': 0x5b, '3': 0x4f,
	'4': 0x66, '5': 0x6d, '6': 0x7d, '7': 0x07,
	'8': 0x7f, '9': 0x6f,
	'a': 0x77, 'b': 0x7c, 'c': 0x39, 'd': 0x5e,
	'e': 0x79, 'f': 0x71,
	'g': 0x6f, // Often same as 9 or custom
	'h': 0x76, 'i': 0x04, // Or 0x06 for '1'
	'j': 0x1e, 'k': 0x76, // Similar to 'h'
	'l': 0x38, 'm': 0x37, // Custom, two 'n's
	'n': 0x54, 'o': 0x5c, 'p': 0x73, 'q': 0x67,
	'r': 0x50, 's': 0x6d, // Same as '5'
	't': 0x78, 'u': 0x3e, 'v': 0x3e, // Same as 'u'
	'w': 0x7e, // Custom, two 'v's
	'x': 0x76, // Similar to 'h'
	'y': 0x6e, 'z': 0x5b, // Same as '2'
	' ': 0x00, // Blank
	'-': 0x40, // Minus
	'_': 0x08, // Underscore (segment D)
	'.': 0x80, // Dot (DP segment only) - special handling if used as char
	'°': 0x63, // Degree symbol (segments A, B, G, F)
}

// New initializes a TM1637 display driver using go-rpio.
// clkPinNumber and dioPinNumber are the BCM GPIO pin numbers.
func New(clkPinNumber, dioPinNumber int) (*TM1637, error) {
	// Open and map memory to access gpio, check for errors
	if err := rpio.Open(); err != nil {
		return nil, fmt.Errorf("failed to open rpio: %w", err)
	}

	clk := rpio.Pin(clkPinNumber)
	dio := rpio.Pin(dioPinNumber)

	d := &TM1637{
		clkPin:         clk,
		dioPin:         dio,
		brightness:     DefaultBrightness,
		colonEnabled:   false,
		delay:          5 * time.Microsecond, // A common delay value
		digitToSegment: make(map[rune]byte),
	}

	// Populate the segment map, allowing for case-insensitivity for hex
	for k, v := range defaultSegmentMap {
		d.digitToSegment[k] = v
		if k >= 'a' && k <= 'f' {
			d.digitToSegment[unicode.ToUpper(k)] = v
		}
	}

	// Set initial pin modes (output, low)
	d.clkPin.Output()
	d.clkPin.Low()
	d.dioPin.Output()
	d.dioPin.Low()

	if err := d.SetBrightness(d.brightness); err != nil {
		// Attempt to close rpio even if setup fails partially
		rpio.Close()
		return nil, fmt.Errorf("failed to set initial brightness: %w", err)
	}
	if err := d.Clear(); err != nil {
		rpio.Close()
		return nil, fmt.Errorf("failed to clear display on init: %w", err)
	}

	return d, nil
}

// sendCommand sends a single command byte to the display.
// Assumes start condition has been sent. Caller must handle stop condition.
func (d *TM1637) sendCommand(cmd byte) error {
	d.start()
	if err := d.writeByte(cmd); err != nil {
		d.stop() // Attempt to stop communication even on error
		return err
	}
	d.stop()
	return nil
}

// SetBrightness sets the display brightness.
// Level should be 0 (dimmest) to 7 (brightest).
func (d *TM1637) SetBrightness(level byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if level > MaxBrightness {
		level = MaxBrightness
	}
	d.brightness = level
	return d.sendDisplayControl()
}

// sendDisplayControl sends the command to control display on/off and brightness.
// Assumes lock is held.
func (d *TM1637) sendDisplayControl() error {
	// Command: 0x80 (base) | 0x08 (display ON) | brightness (0-7)
	// Results in 0x88 (min brightness, on) to 0x8F (max brightness, on)
	cmd := cmdDisplayCtrlBase | 0x08 | d.brightness
	return d.sendCommand(cmd)
}

// Clear clears all digits on the display.
func (d *TM1637) Clear() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	segments := [NumDigits]byte{0x00, 0x00, 0x00, 0x00}
	return d.displayRaw(segments)
}

// SetColon enables or disables the central colon (typically between 2nd and 3rd digits).
// This state is applied on the next call to DisplaySegments or DisplayCharacters.
func (d *TM1637) SetColon(show bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.colonEnabled = show
}

// DisplaySegments displays raw 7-segment data on the 4 digits.
// segmentsData is an array of 4 bytes, each byte representing segment states (DP.G.F.E.D.C.B.A).
// The colon state (if enabled by SetColon) will be applied to the second digit's dot segment.
func (d *TM1637) DisplaySegments(segmentsData [NumDigits]byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	dataToWrite := segmentsData
	if d.colonEnabled {
		// Typically, colon is controlled by the dot segment of the 2nd digit (index 1)
		dataToWrite[1] |= 0x80
	}
	return d.displayRaw(dataToWrite)
}

// DisplayCharacters displays up to 4 characters with optional dots.
// chars: An array of 4 runes to display. Unrecognized chars become blank.
// dots: An array of 4 booleans, true if the dot for the corresponding character should be lit.
// The colon state (if enabled by SetColon) will be applied.
func (d *TM1637) DisplayCharacters(chars [NumDigits]rune, dots [NumDigits]bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	var segmentsData [NumDigits]byte
	for i := 0; i < NumDigits; i++ {
		charToDisplay := chars[i]
		segment, ok := d.digitToSegment[charToDisplay]
		if charToDisplay == '.' { // Special case: if rune is '.', it means only display the dot
			segment = 0x00 // Start with blank
			ok = true
		} else if !ok {
			segment = d.digitToSegment[' '] // Default to blank for unknown characters
		}

		if dots[i] || charToDisplay == '.' {
			segment |= 0x80 // Light up the dot segment (DP)
		}
		segmentsData[i] = segment
	}

	if d.colonEnabled {
		segmentsData[1] |= 0x80 // Apply colon to the dot of the second digit
	}

	return d.displayRaw(segmentsData)
}

// displayRaw sends the 4 segment bytes to the display.
// Assumes lock is held and dataToWrite has colon/dots already incorporated.
func (d *TM1637) displayRaw(dataToWrite [NumDigits]byte) error {
	// Data command: write data to display, auto increment address
	d.start()
	if err := d.writeByte(cmdDataAutoAddr); err != nil {
		d.stop()
		return fmt.Errorf("failed to send data command: %w", err)
	}
	d.stop()

	// Address command: set start address to 0 (0xC0)
	d.start()
	if err := d.writeByte(cmdAddrBase); err != nil {
		d.stop()
		return fmt.Errorf("failed to send address command: %w", err)
	}

	// Send the 4 data bytes for the segments
	for i := 0; i < NumDigits; i++ {
		if err := d.writeByte(dataToWrite[i]); err != nil {
			d.stop()
			return fmt.Errorf("failed to write segment data for digit %d: %w", i, err)
		}
	}
	d.stop()

	// Re-assert display control (brightness/on state)
	return d.sendDisplayControl()
}

// Close turns off the display and releases rpio resources.
func (d *TM1637) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Command to turn display OFF: 0x80 (base) | brightness (0-7). No "ON" bit (0x08).
	cmd := cmdDisplayCtrlBase | d.brightness

	d.start()
	err := d.writeByte(cmd) // Attempt to send off command
	d.stop()                // Ensure stop condition is sent

	// Unmap memory ranges
	// Note: rpio.Close() should be called once at the end of the application,
	// not necessarily per device if multiple rpio devices are used.
	// However, for a single device instance, it's fine here.
	// If managing multiple rpio devices, rpio.Close() should be handled globally.
	// For this library, we assume it's the primary user of rpio or needs to clean up.
	rpio.Close() // This might be too aggressive if other parts of app use rpio.

	if err != nil {
		return fmt.Errorf("failed to send display off command: %w", err)
	}
	return nil
}

// --- Low-level communication methods ---

// start sends the I2C-like start condition.
func (d *TM1637) start() {
	d.dioPin.Output() // Ensure DIO is output before changing state
	d.dioPin.High()
	time.Sleep(d.delay)
	d.clkPin.Output() // Ensure CLK is output
	d.clkPin.High()
	time.Sleep(d.delay)

	d.dioPin.Low()
	time.Sleep(d.delay)

	d.clkPin.Low()
	time.Sleep(d.delay)
}

// stop sends the I2C-like stop condition.
func (d *TM1637) stop() {
	d.clkPin.Output() // Ensure CLK is output
	d.clkPin.Low()
	time.Sleep(d.delay)
	d.dioPin.Output() // Ensure DIO is output
	d.dioPin.Low()
	time.Sleep(d.delay)

	d.clkPin.High()
	time.Sleep(d.delay)

	d.dioPin.High()
	time.Sleep(d.delay)
}

// writeByte sends one byte of data to the TM1637 and waits for ACK.
// Data is sent LSB first.
func (d *TM1637) writeByte(data byte) error {
	d.clkPin.Output() // Ensure CLK is output
	d.dioPin.Output() // Ensure DIO is output

	// Send 8 bits, LSB first
	for i := 0; i < 8; i++ {
		d.clkPin.Low()
		time.Sleep(d.delay)

		if (data & 0x01) == 0x01 {
			d.dioPin.High()
		} else {
			d.dioPin.Low()
		}
		time.Sleep(d.delay) // Data setup time

		d.clkPin.High() // Clock pulse
		time.Sleep(d.delay)

		data >>= 1 // Next bit
	}

	// Wait for ACK:
	// 1. CLK low
	d.clkPin.Low()
	time.Sleep(d.delay)

	// 2. Set DIO to input with pull-up (TM1637 should pull it low for ACK)
	d.dioPin.Input()
	d.dioPin.PullUp() // Enable pull-up resistor
	time.Sleep(d.delay)

	// 3. CLK high to clock out the ACK bit from TM1637
	d.clkPin.High()
	time.Sleep(d.delay)

	// 4. Read ACK bit
	ackState := d.dioPin.Read()

	// 5. CLK low
	d.clkPin.Low()
	time.Sleep(d.delay)

	// 6. Set DIO back to output, low state, ready for next transmission or stop
	d.dioPin.Output()
	d.dioPin.Low()
	// time.Sleep(d.delay) // Optional small delay after restoring DIO

	if ackState == rpio.High { // ACK should be low
		return errors.New("TM1637 NACK (no acknowledge)")
	}
	return nil
}

