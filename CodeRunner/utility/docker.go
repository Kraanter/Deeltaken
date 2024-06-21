package utility

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

var cli, _ = client.NewClientWithOpts(client.FromEnv)

func StartContainer(imageName string, isLocalImage bool) (string, error) {
	// Pull the image
	if !isLocalImage {
		if err := PullImage(imageName); err != nil {
			return "", err
		}
	}

	// Run a image same as command: `docker run -di --rm IMAGENAME`
	resp, err := cli.ContainerCreate(context.Background(), &container.Config{
		Image:       imageName,
		OpenStdin:   true,
		StdinOnce:   true,
		AttachStdin: true,
	}, &container.HostConfig{
		AutoRemove: true,
	}, nil, nil, "")
	if err != nil {
		return "", err
	}

	if err := cli.ContainerStart(context.Background(), resp.ID, container.StartOptions{}); err != nil {
		return "", err
	}

	// Attach to the container
	waiter, err := cli.ContainerAttach(context.Background(), resp.ID, container.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return "", err
	}

	// Do this to prevent the container from closing
	// And to make sure the container is closed when the program is done
	go func() {
		for {
			buf := make([]byte, 1024)
			_, err := waiter.Reader.Read(buf)
			if err != nil {
				break
			}
		}
	}()

	return resp.ID, nil
}

func StopContainer(containerID string) error {
	// Run a image same as command: `docker kill CONTAINERID`
	return cli.ContainerKill(context.Background(), containerID, "KILL")
}

type InputCommandType int
type OutputCommandType int

const (
	RunCode InputCommandType = iota
	RunLSP
	LSPInput
	Stop
)

const (
	StartCodeOutput OutputCommandType = iota
	StartLSPOutput
	CodeOutput
	LSPOutput
	EndCodeOutput
	EndLSPOutput
	Failed
)

type InputCommand struct {
	Payload      string
	InputCommand InputCommandType
}

type OutputCommand struct {
	Payload       string
	OutputCommand OutputCommandType
}

func CreateChannelPair(containerID string) (chan InputCommand, chan OutputCommand) {
	input := make(chan InputCommand)
	output := make(chan OutputCommand)

	go func() {
		defer close(input)
		defer close(output)

		var cmd *exec.Cmd

		// Based on the input command, do something
		for command := range input {
			println("Received command", command.InputCommand, command.Payload)
			switch command.InputCommand {
			case RunCode:
				if cmd != nil {
					output <- OutputCommand{
						Payload:       "A command is already running",
						OutputCommand: Failed,
					}

					continue
				}

				code := command.Payload
				// Run the code
				err := func() error {
					containerCmd, err := RunCodeOnContainer(containerID, code)
					if err != nil {
						fmt.Println("Error preparing to run code on container:", err.Error())
						return err
					}

					// Create a pipe to the stdout of the command
					stdout, err := containerCmd.StdoutPipe()
					if err != nil {
						println("Error creating stdout pipe", err.Error())
						return err
					}

					// Create a pipe to the stderr of the command
					stderr, err := containerCmd.StderrPipe()
					if err != nil {
						println("Error creating stderr pipe", err.Error())
						return err
					}

					cmd = containerCmd

					// Start the command
					if err := cmd.Start(); err != nil {
						println("Error starting command", err.Error())
						return err
					}

					output <- OutputCommand{
						Payload:       "Code is running",
						OutputCommand: StartCodeOutput,
					}

					// Create a channel to return the output
					cmdOutput := make(chan string)
					go func() {
						// Read the output of the command
						go func() {
							buf := make([]byte, 1024)
							for {
								n, err := stdout.Read(buf)
								if err != nil {
									break
								}
								cmdOutput <- string(buf[:n])
							}
							output <- OutputCommand{
								Payload:       "",
								OutputCommand: EndCodeOutput,
							}
							close(cmdOutput)
						}()

						// Read the error of the command
						go func() {
							buf := make([]byte, 1024)
							for {
								n, err := stderr.Read(buf)
								if err != nil {
									break
								}
								cmdOutput <- "error: " + string(buf[:n])
							}
						}()

						// Wait for the command to finish
						for text := range cmdOutput {
							output <- OutputCommand{
								Payload:       text,
								OutputCommand: CodeOutput,
							}
						}

						// Wait for the command to finish
						cmd.Wait()

						cmd = nil
					}()

					return nil
				}()
				if err != nil {
					fmt.Println("Error running command: remote", err.Error())
					output <- OutputCommand{
						Payload:       "Error running command",
						OutputCommand: Failed,
					}

					continue
				}
			case Stop:
				if cmd == nil {
					continue
				}

				err := cmd.Process.Kill()
				if err != nil {
					println("Error stopping command", err.Error())
				}

				// Wait for the command to finish
				cmd.Wait()
			case RunLSP:
				if cmd != nil {
					output <- OutputCommand{
						Payload:       "A command is already running",
						OutputCommand: Failed,
					}
					continue
				}

				println("Running LSP")

				// Run the LSP
				err := func() error {
					containerCmd, inputLSP, outputLSP, err := RunLSPOnContainer(containerID)
					if err != nil {
						fmt.Println("Error preparing to run LSP on container:", err.Error())
						return err
					}

					output <- OutputCommand{
						Payload:       "LSP is running",
						OutputCommand: StartLSPOutput,
					}

					cmd = containerCmd

					go func() {
						buf := make([]byte, 1024)
						for {
							n, err := outputLSP.Read(buf)
							if err != nil {
								break
							}

							println("Got output from LSP: ", string(buf[:n]))

							output <- OutputCommand{
								Payload:       string(buf[:n]),
								OutputCommand: LSPOutput,
							}
						}
					}()

					// Any input on the input channel should be sent to the LSP
					// Any output from the LSP should be sent to the output channel
					println("Starting LSP input/output loop")
					for {
						println("Waiting for input to send to LSP")
						lspInput := <-input

						fmt.Printf("Got input: %v\n", lspInput)
						if lspInput.InputCommand != LSPInput {
							continue
						}
						input := lspInput.Payload
						fmt.Println("Sending input to LSP: ", input)

						_, err = fmt.Fprintf(inputLSP, "Content-Length: %d\r\n\r\n", len(input))
						_, err := inputLSP.Write([]byte(input))
						if err != nil {
							fmt.Println("Error writing to LSP", err.Error())
							output <- OutputCommand{
								Payload:       "Error writing to LSP",
								OutputCommand: Failed,
							}
							break
						}
					}

					return nil
				}()
				if err != nil {
					fmt.Println("Error running command: remote", err.Error())
					output <- OutputCommand{
						Payload:       "Error running command",
						OutputCommand: Failed,
					}
					continue
				}
			}
		}
	}()

	return input, output
}

func RunCodeOnContainer(containerID, code string) (*exec.Cmd, error) {
	// Run this command: docker exec -i container_id2 sh -c 'cat > ./bar/foo.txt' < ./input.txt
	cmd := exec.Command("docker", "exec", "-i", containerID, "sh", "-c", "cat > /input.txt")

	// Create a pipe to the stdin of the command
	stdin, err := cmd.StdinPipe()
	if err != nil {
		println("Error creating stdin pipe", err.Error())
		return nil, err
	}

	// Write the code to the stdin of the command
	if _, err := stdin.Write([]byte(code)); err != nil {
		println("Error writing to stdin", err.Error())
		return nil, err
	}

	// Close the stdin pipe
	if err := stdin.Close(); err != nil {
		println("Error closing stdin", err.Error())
		return nil, err
	}

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		fmt.Println(fmt.Sprint(err) + ": " + stderr.String())
		return nil, err
	}

	cmd = exec.Command("docker", "exec", containerID, "/source/script.sh")

	return cmd, nil
}

func RunLSPOnContainer(containerID string) (*exec.Cmd, io.Writer, io.Reader, error) {
	cmd := exec.Command("docker", "exec", "-i", containerID, "csharp-ls")
	// cmd := exec.Command("docker", "exec", "-i", containerID, "cat")

	// Create a pipe to the stdin of the command
	stdin, err := cmd.StdinPipe()
	if err != nil {
		println("Error creating stdin pipe", err.Error())
		return nil, nil, nil, err
	}

	// Create a pipe to the stdout of the command
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		println("Error creating stdout pipe", err.Error())
		return nil, nil, nil, err
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		println("Error starting command", err.Error())
		return nil, nil, nil, err
	}

	// Create reader and writer
	reader := io.Reader(stdout)
	writer := io.Writer(stdin)

	return cmd, writer, reader, nil
}

func IsImagePresent(imageName string) bool {
	list, err := cli.ImageList(context.Background(), image.ListOptions{})
	if err != nil {
		panic(err)
	}
	for _, ims := range list {
		for _, tag := range ims.RepoTags {
			if strings.HasPrefix(tag, imageName) {
				return true
			}
		}
	}

	return false
}

func PullImage(imageName string) error {
	// Always pull the image to make sure it is up to date
	_, err := cli.ImagePull(context.Background(), imageName, image.PullOptions{})

	// Wait for any image to be usable
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		for !IsImagePresent(imageName) {
			time.Sleep(1 * time.Second)
		}
		wg.Done()
	}(&wg)
	wg.Wait()

	return err
}
