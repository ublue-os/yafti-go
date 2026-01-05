package main

// Proof of Concept: Fyne-based native GUI for yafti-go
// This demonstrates the core features without the server/browser approach

import (
	"bufio"
	"fmt"
	"os/exec"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// Mock config structures (would import from existing config package)
type Action struct {
	ID          string
	Title       string
	Description string
	Default     bool
	Script      string
}

type Screen struct {
	Title   string
	Actions []Action
}

type Config struct {
	Screens []Screen
}

// Example config for demonstration
var mockConfig = Config{
	Screens: []Screen{
		{
			Title: "Deck Utils",
			Actions: []Action{
				{
					ID:          "decky-loader",
					Title:       "Decky Loader",
					Description: "A plugin loader for the Steam Deck",
					Default:     false,
					Script:      "echo 'Installing Decky Loader...'; sleep 2; echo 'Done!'",
				},
				{
					ID:          "steam-tweaks",
					Title:       "Steam Tweaks",
					Description: "Apply performance tweaks for Steam",
					Default:     true,
					Script:      "echo 'Applying Steam tweaks...'; sleep 1; echo 'Complete!'",
				},
			},
		},
		{
			Title: "Additional Tweaks",
			Actions: []Action{
				{
					ID:          "input-group",
					Title:       "Add input group",
					Description: "Add input group to current user",
					Default:     false,
					Script:      "echo 'Adding user to input group...'; sleep 1; echo 'User added!'",
				},
			},
		},
	},
}

func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("Yafti-Go - Native GUI")
	myWindow.Resize(fyne.NewSize(800, 600))

	// Show welcome screen
	showWelcomeScreen(myWindow)

	myWindow.ShowAndRun()
}

// Welcome screen - lists all available action groups
func showWelcomeScreen(w fyne.Window) {
	title := widget.NewLabel("Welcome to Bazzite!")
	title.Alignment = fyne.TextAlignCenter
	title.TextStyle = fyne.TextStyle{Bold: true}

	subtitle := widget.NewLabel("Please select a configuration screen to begin")
	subtitle.Alignment = fyne.TextAlignCenter

	// Create buttons for each screen
	buttons := make([]fyne.CanvasObject, 0)
	for i, screen := range mockConfig.Screens {
		screenIndex := i // Capture for closure
		btn := widget.NewButton(screen.Title, func() {
			showActionScreen(w, screenIndex)
		})
		buttons = append(buttons, btn)
	}

	content := container.NewVBox(
		layout.NewSpacer(),
		container.NewCenter(container.NewVBox(
			title,
			subtitle,
		)),
		layout.NewSpacer(),
		container.NewCenter(container.NewVBox(buttons...)),
		layout.NewSpacer(),
		container.NewCenter(widget.NewLabel("Bazzite Portal • Powered by Yafti-Go")),
	)

	w.SetContent(container.NewPadded(content))
}

// Action screen - shows checkboxes for each action
func showActionScreen(w fyne.Window, screenIndex int) {
	screen := mockConfig.Screens[screenIndex]

	title := widget.NewLabel(screen.Title)
	title.TextStyle = fyne.TextStyle{Bold: true}

	subtitle := widget.NewLabel("Select actions to execute, then click Run")

	// Track which actions are selected
	selectedActions := make(map[string]bool)

	// Create checkboxes for each action
	actionWidgets := make([]fyne.CanvasObject, 0)
	for _, action := range screen.Actions {
		act := action // Capture for closure
		selectedActions[act.ID] = act.Default

		check := widget.NewCheck(act.Title, func(checked bool) {
			selectedActions[act.ID] = checked
		})
		check.Checked = act.Default

		desc := widget.NewLabel(act.Description)
		desc.Wrapping = fyne.TextWrapWord

		actionCard := container.NewVBox(
			check,
			desc,
			widget.NewSeparator(),
		)
		actionWidgets = append(actionWidgets, actionCard)
	}

	// Buttons
	backBtn := widget.NewButton("← Back", func() {
		showWelcomeScreen(w)
	})

	runBtn := widget.NewButton("Run Selected Actions", func() {
		// Collect selected actions
		toRun := make([]Action, 0)
		for _, action := range screen.Actions {
			if selectedActions[action.ID] {
				toRun = append(toRun, action)
			}
		}

		if len(toRun) == 0 {
			dialog.ShowInformation("No Actions", "Please select at least one action to run", w)
			return
		}

		executeActions(w, toRun, screenIndex)
	})

	buttonBar := container.NewHBox(
		backBtn,
		layout.NewSpacer(),
		runBtn,
	)

	content := container.NewBorder(
		container.NewVBox(title, subtitle, widget.NewSeparator()),
		buttonBar,
		nil,
		nil,
		container.NewVScroll(container.NewVBox(actionWidgets...)),
	)

	w.SetContent(container.NewPadded(content))
}

// Execute actions and show output
func executeActions(w fyne.Window, actions []Action, screenIndex int) {
	title := widget.NewLabel("Executing Actions...")
	title.TextStyle = fyne.TextStyle{Bold: true}

	outputText := widget.NewLabel("")
	outputText.Wrapping = fyne.TextWrapWord

	scroll := container.NewVScroll(outputText)
	scroll.SetMinSize(fyne.NewSize(700, 400))

	progress := widget.NewProgressBar()

	backBtn := widget.NewButton("← Back", func() {
		showActionScreen(w, screenIndex)
	})
	backBtn.Disable()

	content := container.NewBorder(
		container.NewVBox(title, progress),
		backBtn,
		nil,
		nil,
		scroll,
	)

	w.SetContent(container.NewPadded(content))

	// Execute actions asynchronously
	go func() {
		output := ""
		for i, action := range actions {
			progress.SetValue(float64(i) / float64(len(actions)))

			output += fmt.Sprintf("\n=== Running: %s ===\n", action.Title)
			outputText.SetText(output)

			// Execute the script
			cmd := exec.Command("sh", "-c", action.Script)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				output += fmt.Sprintf("Error: %v\n", err)
				outputText.SetText(output)
				continue
			}

			if err := cmd.Start(); err != nil {
				output += fmt.Sprintf("Error starting: %v\n", err)
				outputText.SetText(output)
				continue
			}

			// Stream output
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				line := scanner.Text()
				output += line + "\n"
				outputText.SetText(output)
				scroll.ScrollToBottom()
			}

			if err := cmd.Wait(); err != nil {
				output += fmt.Sprintf("Error: %v\n", err)
			} else {
				output += "✓ Complete\n"
			}
			outputText.SetText(output)
		}

		progress.SetValue(1.0)
		output += "\n=== All actions complete ===\n"
		outputText.SetText(output)
		backBtn.Enable()
	}()
}
