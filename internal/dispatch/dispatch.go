package dispatch

import (
	"fmt"
	"os"
	"tiddlybench-cli/internal/apicall"
	"tiddlybench-cli/internal/cliflags"
	"tiddlybench-cli/internal/clipboard"
	"tiddlybench-cli/internal/config"
	"tiddlybench-cli/internal/editor"
	"tiddlybench-cli/internal/logger"
	"tiddlybench-cli/internal/piper"
	prompter "tiddlybench-cli/internal/prompt"
)

var (
	cfg    *config.Config
	pipe   *piper.Pipe
	prompt *prompter.Prompt
	api    *apicall.APICall
)

// Run the right activity for the app
func Run(log logger.Logger) {

	cfg = config.New(log)
	pipe = piper.New(log)
	api = apicall.New(log, cfg)
	prompt = prompter.New(log, cfg)

	cliflags.Setup()

	if pipe.IsPipeSet() {
		// Piping breaks the ability to use the prompt
		checkRequirementsForPipe()
	}

	if cliflags.ShouldPromptForConfig() || !cfg.IsConfigFileSet() {
		// Prompt to configure the username/password
		prompt.PromptForConfig()
		os.Exit(0)
	}

	if cfg.IsConfigFileSet() {
		// Verify the password and test the connection
		verifyPasswordAndConnection()

		tiddlerTitle := getTiddlerTitleFromFlags()
		if tiddlerTitle == "" && !pipe.IsPipeSet() {
			// Prompt for the tiddler title
			tiddlerTitle = prompt.PromptTiddlerTitle(tiddlerTitle)
		}

		currentTiddler := api.GetTiddlerByName(tiddlerTitle)

		tidtext := ""
		if pipe.IsPipeSet() {
			tidtext = pipe.Get()
		} else if cliflags.IsAddTextSet() {
			tidtext = cliflags.GetAddText()
		} else if cliflags.ShouldPaste() {
			// Use the clipboard contents for the tiddler
			tidtext = clipboard.Paste(log)
		} else {
			// Prompt the user for the tiddler
			tidtext = prompt.PromptTiddlerText()
		}

		if cliflags.ShouldUseEditor() {
			editorSetting := cfg.Get(config.CKTextEditorKey + "." + config.CKTextEditorDefaultKey)
			argsSetting := cfg.Get(config.CKTextEditorKey + "." + config.CKTextEditorArgsKey)
			textFromEditor, eerr := editor.Edit(tidtext, editorSetting, argsSetting)
			if eerr != nil {
				fmt.Println("Unable to use the editor.")
				fmt.Println(eerr)
				os.Exit(1)
			}
			tidtext = textFromEditor
		}

		// Wrap the text in the selected block
		tidtext = wrapTextInBlock(tidtext)

		ok := false
		method := ""
		if currentTiddler.Title != "" {
			method = "update"
			fulltext := currentTiddler.Text + "\n" + tidtext
			ok = api.UpdateTiddler(currentTiddler.Title, fulltext)
		} else {
			method = "add"
			creator := cfg.Get(config.CKUsername)
			ok = api.AddNewTiddler(tiddlerTitle, creator, tidtext)
		}

		if ok {
			fmt.Println("Success.")
			fmt.Println("'" + tiddlerTitle + "' was " + method + "ed.")
		} else {
			fmt.Println("Failed to " + method + " '" + tiddlerTitle + "'.")
		}
	}
}

func wrapTextInBlock(txt string) string {
	block := cliflags.GetSelectedBlock()
	beginBlock := cfg.GetNested(config.CKBlocks, block, config.CKBegin)
	endBlock := cfg.GetNested(config.CKBlocks, block, config.CKEnd)
	return beginBlock + txt + endBlock
}

func verifyPasswordAndConnection() {

	if !cfg.IsPasswordSaved() {
		// Password is not saved
		if cliflags.IsPasswordSet() {
			// The password flag is set so lets use that
			passwordFromFlag := cliflags.GetPassword()
			cfg.Set(config.CKPassword, passwordFromFlag)
		} else if !pipe.IsPipeSet() {
			// Prompt for a password
			password := prompt.PromptForPassword()
			cfg.Set(config.CKPassword, password)
		}
	}

	if !api.IsValidConnection() {
		url := cfg.Get(config.CKURL)
		username := cfg.Get(config.CKUsername)
		fmt.Println("Connection Error. The url, username, or password is incorrect")
		fmt.Println("Configured URL: " + url)
		fmt.Println("Configured Username: " + username)
		fmt.Println("Run 'tb -c' to reconfigure or try a different password.")
		os.Exit(1)
	}
}

func checkRequirementsForPipe() {
	// Pipe is set, so can't use
	// any of the prompt methods

	// Must be configured
	requireConfigFile()

	// Must have password
	requirePasswordFlag()

	// Must have Inbox, Journal, or -t flag
	requireTiddlerTitleFlag()
}

func getTiddlerTitleFromFlags() string {
	tiddlerTitle := cliflags.GetTiddlerTitle()
	if tiddlerTitle != "" {
		return tiddlerTitle
	}

	sendTo := cliflags.GetSendTo()
	if sendTo == "" {
		// No destination flag is set so use the config default
		sendTo = cfg.GetNested(config.CKDestinations, config.CKDefaultDestination)
	}

	if sendTo != "" {
		return cfg.GetNested(config.CKDestinations, sendTo, config.CKTitleTemplate)
	}
	return ""
}

func requireConfigFile() {

	if !cfg.IsConfigFileSet() {
		fmt.Println("Config file has not been set.")
		fmt.Println("Run with -c option to configure")
		os.Exit(1)
	}
}

func requirePasswordFlag() {
	if !cfg.IsPasswordSaved() && !cliflags.IsPasswordSet() {
		fmt.Println("Password is required, but it is not saved in the config file.")
		fmt.Println("Add the password to the command line arguments: tb --password 'YourPass'")
		os.Exit(1)
	}
}

func requireTiddlerTitleFlag() {
	hasTiddlerTitle := cliflags.GetTiddlerTitle() != ""
	hasSendTo := cliflags.GetSendTo() != ""
	if hasTiddlerTitle && hasSendTo {
		fmt.Println("You have set too many destination tiddlers.")
		fmt.Println("Include just one of -i, -j, or -t.")
		os.Exit(1)
	}
	if !hasTiddlerTitle && !hasSendTo {
		fmt.Println("You must include a destination tiddler.")
		fmt.Println("Include -i (inbox), -j (journal), or -t (custom tiddler).")
		os.Exit(1)
	}
}
