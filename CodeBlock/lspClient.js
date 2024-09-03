export class LSPClient {
  connection;
  fileUri;

  constructor(fileUri, language) {
    if (!language)
      throw new Error('Language must be provided to create LSPClient instance');
    if (!fileUri)
      console.warn('File URI is not provided. This may cause issues with the LSP server');

    this.connection = new WebSocket('ws://localhost:8080/lsp/' + language);
    this.connection.onopen = () => {
      console.log('Connected to the server');
    };
    this.connection.onmessage = (message) => {
      console.log('Received message:', message.data);
    };

    this.fileUri = fileUri;
  }

  send(message) {
    try {
      message = JSON.stringify(message);
    } catch (e) {
      console.warn("Could not stringify message: ", e);
    }

    this.connection.send(message);
  }
}

