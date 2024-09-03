package endpoints

import (
	"fmt"
	"net/http"

	"github.com/Windesheim-HBO-ICT/Deeltaken/CodeRunner/lxu"
	"github.com/Windesheim-HBO-ICT/Deeltaken/CodeRunner/runner"
	"github.com/Windesheim-HBO-ICT/Deeltaken/CodeRunner/utility"
)

func lspWebsocket(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	language := r.URL.Query().Get("language")
	if language == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	if _, ok := runner.LangDefs[language]; !ok {
		http.Error(w, "Language '"+language+"' is not supported", http.StatusUnprocessableEntity)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Could not upgrade connection", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	languageDef, err := runner.GetLangDef(language)
	if err != nil {
		fmt.Println("Could not get language definition", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	runner, err := lxu.LXUM.CreateContainerRef(languageDef)
	if err != nil {
		fmt.Println("Could not start container", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer println("Destroying container")
	defer runner.Destroy()

	lspInput, lspOutput, err := runner.StreamLSP()
	if err != nil {
		fmt.Println("Could not stream LSP", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	utility.BindInputChannelToWebsocket(conn, lspInput, func() {
		runner.Destroy()
	})
	utility.BindWebsocketToOutputChannel(conn, lspOutput)

}
