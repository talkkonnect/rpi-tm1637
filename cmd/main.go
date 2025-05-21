package main

import (
	//"fmt"
	"log"
	"time"

	// Assuming your tm1637 library is in a 'tm1637' directory
	// relative to your project's go.mod file, or you've published it
	// and can import it via its module path.
	// For local development, if main.go and tm1637/tm1637.go are in the same parent directory:
	// go mod init my_project_name
	// then import "my_project_name/tm1637"
	tm1637 "github.com/rpi-tm1637" // IMPORTANT: Replace 'your_module_path' with your actual module path
)

func main() {
	// --- Configuration ---
	// !!! IMPORTANT: Replace with your actual BCM GPIO pin numbers !!!
	clkPin := 23 // Example BCM pin for CLK (e.g., physical pin 16 on RPi)
	dioPin := 24 // Example BCM pin for DIO (e.g., physical pin 18 on RPi)

	log.Println("Initializing TM1637 display...")

	// Initialize the display using BCM pin numbers
	display, err := tm1637.New(clkPin, dioPin)
	if err != nil {
		log.Fatalf("Failed to initialize TM1637 display: %v. Ensure you are running with necessary permissions (e.g., sudo).", err)
	}
	// Defer Close to ensure rpio resources are released and display is turned off
	defer func() {
		log.Println("Closing TM1637 display...")
		if err := display.Close(); err != nil {
			log.Printf("Error closing display: %v", err)
		}
		log.Println("Display closed.")
	}()

	log.Println("TM1637 display initialized successfully.")

	// --- Example 1: Display numbers with colon ---
	log.Println("Example 1: Displaying '12:34'")
	display.SetColon(false)
	err = display.DisplayCharacters(
		[4]rune{'1', '2', '3', '4'},
		[4]bool{false, false, false, false}, // No individual dots
	)
	if err != nil {
		log.Printf("Error displaying '12:34': %v", err)
	}
	time.Sleep(3 * time.Second)

	// --- Example 2: Display "HELP" with a dot on 'P' ---
	log.Println("Example 2: Displaying 'H.E.L.P.' (dots on each char)")
	display.SetColon(false) // Turn off colon for this example
	err = display.DisplayCharacters(
		[4]rune{'h', 'e', 'l', 'p'},
		[4]bool{false, false, true, true}, // Dots on H, E, L, P
	)
	if err != nil {
		log.Printf("Error displaying 'H.E.L.P.': %v", err)
	}
	time.Sleep(3 * time.Second)

	// --- Example 3: Display "AbCd" ---
	log.Println("Example 3: Displaying 'AbCd'")
	display.SetColon(false)
	err = display.DisplayCharacters(
		[4]rune{'A', 'b', 'C', 'd'},
		[4]bool{false, false, false, false},
	)
	if err != nil {
		log.Printf("Error displaying 'AbCd': %v", err)
	}
	time.Sleep(3 * time.Second)

	// --- Example 4: Scrolling text (simple marquee) ---
	log.Println("Example 4: Scrolling 'HELLO PI '")
	display.SetColon(false)
	textToScroll := "hEllo pi    " // Add padding for smoother scroll out
	runes := []rune(textToScroll)
	scrollDelay := 500 * time.Millisecond

	for i := 0; i <= len(runes)-tm1637.NumDigits; i++ {
		charsToShow := [tm1637.NumDigits]rune{}
		dotsToShow := [tm1637.NumDigits]bool{} // All false

		// Prepare the 4 characters for the current frame
		for j := 0; j < tm1637.NumDigits; j++ {
			if i+j < len(runes) {
				charsToShow[j] = runes[i+j]
			} else {
				charsToShow[j] = ' ' // Pad with spaces if text is shorter
			}
		}
		err = display.DisplayCharacters(charsToShow, dotsToShow)
		if err != nil {
			log.Printf("Error scrolling text: %v", err)
			break
		}
		time.Sleep(scrollDelay)
	}
	time.Sleep(1 * time.Second) // Pause after scroll

	// --- Example 5: Brightness sweep ---
	log.Println("Example 5: Brightness sweep from 0 to 7")
	display.SetColon(false) // Turn colon on for this
	for bright := byte(0); bright <= tm1637.MaxBrightness; bright++ {
		log.Printf("Setting brightness to %d", bright)
		if err := display.SetBrightness(bright); err != nil {
			log.Printf("Error setting brightness to %d: %v", bright, err)
			continue
		}
		// Display something to see the brightness change
		err = display.DisplayCharacters([4]rune{'8', '8', '8', '8'}, [4]bool{false, true, false, true})
		if err != nil {
			log.Printf("Error displaying for brightness test: %v", err)
		}
		time.Sleep(700 * time.Millisecond)
	}
	// Reset to a default brightness
	log.Println("Resetting brightness to default.")
	if err := display.SetBrightness(tm1637.DefaultBrightness); err != nil { // Assuming DefaultBrightness is exported or use a value
		log.Printf("Error resetting brightness: %v", err)
	}
	time.Sleep(2 * time.Second)

	// --- Example 6: Display raw segments (e.g., custom pattern or animation frame) ---
	log.Println("Example 6: Displaying raw segments (all segments on)")
	display.SetColon(false) // Explicitly manage dot for segment display
	allOn := byte(0xFF)     // All segments + dot
	segmentsData := [tm1637.NumDigits]byte{allOn, allOn, allOn, allOn}
	if err := display.DisplaySegments(segmentsData); err != nil {
		log.Printf("Error displaying raw segments: %v", err)
	}
	time.Sleep(3 * time.Second)

	// --- Example 7: Displaying numbers with leading zeros (as characters) ---
	log.Println("Example 7: Displaying '0042'")
	display.SetColon(false)
	err = display.DisplayCharacters(
		[4]rune{'0', '0', '4', '2'},
		[4]bool{false, false, false, false},
	)
	if err != nil {
		log.Printf("Error displaying '0042': %v", err)
	}
	time.Sleep(3 * time.Second)
	
	// --- Example 8: Displaying special characters and dots ---
	log.Println("Example 8: Displaying '-.°. '")
	display.SetColon(false)
	err = display.DisplayCharacters(
		[4]rune{'-', '.', '°', ' '}, // Display '-', a dot, degree symbol, and a blank
		[4]bool{true, false, true, false}, // Add dot to '-', no extra dot to '.', add dot to '°'
	)
	if err != nil {
		log.Printf("Error displaying special chars: %v", err)
	}
	time.Sleep(3 * time.Second)


	// --- Final Step: Clear display before closing (optional, Close() also turns it off) ---
	log.Println("Clearing display...")
	if err := display.Clear(); err != nil {
		log.Printf("Error clearing display: %v", err)
	}
	time.Sleep(1 * time.Second)

	log.Println("TM1637 demo finished.")
}

