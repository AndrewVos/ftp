package ftp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Client struct {
	Host       string
	Port       int
	Username   string
	Password   string
	Connection net.Conn
	reader     *bufio.Reader
}

type Response struct {
	Code    int
	Message string
}

func (r Response) Error() error {
	return errors.New(r.Message)
}

type Reader struct {
	Connection net.Conn
	Client     *Client
}

func (r *Reader) Read(buf []byte) (int, error) {
	n, err := r.Connection.Read(buf)
	return n, err
}

func (r *Reader) Close() error {
	connectionCloseError := r.Connection.Close()
	response, parseResponseError := r.Client.parseResponse()

	if response.Code != 226 {
		return response.Error()
	}

	if connectionCloseError != nil {
		return connectionCloseError
	}
	if parseResponseError != nil {
		return parseResponseError
	}

	return nil
}

type Entry struct {
	Name      string
	Directory bool
	Link      bool
}

func (f *Client) Connect() error {
	connection, err := net.Dial("tcp", f.Host+":"+strconv.Itoa(f.Port))
	if err != nil {
		return err
	}
	f.Connection = connection
	f.reader = bufio.NewReader(f.Connection)
	response, err := f.parseResponse()
	if err != nil {
		return err
	}
	if response.Code != 220 {
		return response.Error()
	}

	return nil
}

func (f *Client) Close() error {
	return f.Connection.Close()
}

func (f *Client) List(path string) ([]Entry, error) {
	reader, err := f.dataCmd(fmt.Sprintf("LIST %s", path), 150)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	bufferedReader := bufio.NewReader(reader)
	var entries []Entry
	for {
		line, err := bufferedReader.ReadString('\n')
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		parts := strings.Split(line, " ")

		entry := Entry{}
		entry.Directory = strings.HasPrefix(parts[0], "d")
		entry.Link = strings.HasPrefix(parts[0], "l")

		if entry.Link {
			entry.Name = strings.Trim(parts[len(parts)-3], "\r\n")
		} else {
			entry.Name = strings.Trim(parts[len(parts)-1], "\r\n")
		}
		entries = append(entries, entry)
	}
	f.parseResponse()

	return entries, nil
}

func (f *Client) Retr(path string) (*Reader, error) {
	_, err := f.cmd("TYPE I", []int{200})
	if err != nil {
		return nil, err
	}
	connection, err := f.dataCmd(fmt.Sprintf("RETR %s", path), 150)
	if err != nil {
		return nil, err
	}
	return &Reader{
		Client:     f,
		Connection: connection,
	}, nil
}

func (f *Client) parseResponse() (Response, error) {
	var code int
	var responses []string

	for {
		response, err := f.reader.ReadString('\n')
		if err != nil {
			return Response{}, err
		}
		responses = append(responses, response)

		code, err = strconv.Atoi(strings.Trim(response, " ")[0:3])
		if err != nil {
			return Response{}, err
		}
		lastResponse, err := regexp.MatchString("^\\d{3} ", response)
		if err != nil {
			return Response{}, err
		}
		if lastResponse {
			break
		}
	}
	fullResponse := strings.Trim(strings.Join(responses, "\n"), "\n")
	debugPrint(fullResponse)
	return Response{
		Code:    code,
		Message: fullResponse,
	}, nil
}

func (f *Client) Login() error {
	_, err := f.cmd(fmt.Sprintf("USER %s", f.Username), []int{230, 331, 332})
	if err != nil {
		return err
	}
	_, err = f.cmd(fmt.Sprintf("PASS %s", f.Password), []int{230, 202})
	return err
}

func (f *Client) dataCmd(command string, initialResponseCode int) (net.Conn, error) {
	port, err := f.initiatePassiveMode()
	if err != nil {
		return nil, err
	}
	passiveConnection, err := net.Dial("tcp", fmt.Sprintf("%s:%d", f.Host, port))
	if err != nil {
		return nil, err
	}

	_, err = f.cmd(command, []int{initialResponseCode})
	if err != nil {
		passiveConnection.Close()
		return nil, err
	}
	return passiveConnection, nil
}

func (f *Client) initiatePassiveMode() (int, error) {
	response, err := f.cmd("PASV", []int{227})
	if err != nil {
		return 0, err
	}

	invalidResponseError := errors.New(fmt.Sprintf("invalid EPSV response format %q", response.Message))
	regex := regexp.MustCompile("\\(([0-9,]+)\\)")
	matches := regex.FindStringSubmatch(response.Message)
	if len(matches) != 2 {
		return 0, invalidResponseError
	}
	numbers := strings.Split(matches[1], ",")
	p1, err := strconv.Atoi(numbers[len(numbers)-2])
	if err != nil {
		return 0, err
	}
	p2, err := strconv.Atoi(numbers[len(numbers)-1])
	if err != nil {
		return 0, err
	}

	return p1*256 + p2, nil
}

func (f *Client) cmd(command string, expectedCodes []int) (Response, error) {
	debugPrint(command)
	_, err := f.Connection.Write([]byte(command + "\n"))
	if err != nil {
		return Response{}, err
	}
	response, err := f.parseResponse()
	if err != nil {
		return response, err
	}
	for _, expectedCode := range expectedCodes {
		if response.Code == expectedCode {
			return response, nil
		}
	}
	return response, response.Error()
}

func debugPrint(s string) {
	if os.Getenv("DEBUG") != "" {
		fmt.Println(s)
	}
}
