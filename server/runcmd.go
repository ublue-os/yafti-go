package server

import (
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/Zeglius/yafti-go/config"
)

// Execute a cmd in a terminal.
func runCmd(cmd string) {

	cmd = strings.Trim(cmd, "\n\r\t")

	tempfile, err := os.CreateTemp("/tmp", "*.sh")
	if err != nil {
		log.Println("error: couldnt create temporary file for script")
		return
	}

	defer func() {
		_ = tempfile.Close()
		_ = os.Remove(tempfile.Name())
		log.Println("temporary file removed")
	}()

	// Write the cmd into the temporary file
	if !strings.HasPrefix(cmd, "#!") {
		log.Println("adding shebang")
		cmd = "#!/bin/bash\n" + cmd
	}
	_, err = tempfile.WriteString(cmd)
	if err != nil {
		return
	}
	tempfile.Close()
	log.Println("new file created: " + tempfile.Name())

	// Make the temporary file executable
	err = os.Chmod(tempfile.Name(), 0755)
	if err != nil {
		log.Println("error: couldnt make temporary file executable")
		return
	}

	// Execute it.
	log.Println("executing temporary file" + tempfile.Name())
	com := exec.Command("ptyxis", "-s", "--", tempfile.Name())

	if err := com.Run(); err != nil {
		log.Printf("error: couldnt execute temporary file: %v", err)
	}

	return
}

func ExecuteActionWithId(actionId string) {
	if actions, ok := config.ConfStatus.GetActionsByIds([]string{actionId}); ok {
		runCmd(actions[0].Script)
	}
}
