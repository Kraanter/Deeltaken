package lxu

import (
	"fmt"

	"github.com/Windesheim-HBO-ICT/Deeltaken/CodeRunner/utility"
)

type LXUReference struct {
	id     string
	output chan utility.OutputCommand
	input  chan utility.InputCommand
	parent *LXUContainer
}

func (lxu *LXUReference) RunCode(code string) (string, error) {
	outputChan, err := lxu.StreamCode(code)
	if err != nil {
		return "", err
	}

	output := ""
	for {
		line, ok := <-outputChan
		if !ok {
			break
		}
		output += line
	}

	return output, nil
}

func (lxu *LXUReference) StopCodeStream() {
	lxu.input <- utility.InputCommand{
		InputCommand: utility.Stop,
	}
}

func (lxu *LXUReference) Destroy() {
	lxu.parent.RemoveReference(lxu.id)
}

func (lxu *LXUReference) StreamCode(code string) (chan string, error) {
	lxu.input <- utility.InputCommand{
		InputCommand: utility.RunCode,
		Payload:      code,
	}

	outputChannel := make(chan string)

	output := <-lxu.output
	switch output.OutputCommand {
	case utility.StartCodeOutput:
		go func() {
			for {
				output := <-lxu.output
				if output.OutputCommand == utility.EndCodeOutput {
					close(outputChannel)
					return
				} else if output.OutputCommand == utility.Failed {
					close(outputChannel)
					return
				} else if output.OutputCommand == utility.CodeOutput {
					outputChannel <- output.Payload
				}
			}
		}()
		return outputChannel, nil
	case utility.Failed:
		return nil, fmt.Errorf(output.Payload)
	default:
		return nil, fmt.Errorf("Unexpected output command: %d", output.OutputCommand)
	}
}

func (lxu *LXUReference) StreamLSP() (chan string, chan string, error) {
	lxu.input <- utility.InputCommand{
		InputCommand: utility.RunLSP,
	}

	inputChannel := make(chan string)
	outputChannel := make(chan string)

	go func() {
		for {
			select {
			case input := <-inputChannel:
				println("Sending input to LSP", input)
				lxu.input <- utility.InputCommand{
					InputCommand: utility.LSPInput,
					Payload:      input,
				}
			}
		}
	}()

	go func() {
		for {
			output := <-lxu.output
			println("Received output from LSP", output.OutputCommand, output.Payload)
			if output.OutputCommand == utility.EndLSPOutput {
				close(outputChannel)
				return
			} else if output.OutputCommand == utility.Failed {
				close(outputChannel)
				return
			} else if output.OutputCommand == utility.LSPOutput {
				outputChannel <- output.Payload
			} else if output.OutputCommand == utility.StartLSPOutput {
				fmt.Println("LSP started")
				outputChannel <- output.Payload
				continue
			}
		}
	}()

	return inputChannel, outputChannel, nil
}
