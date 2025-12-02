.PHONY: install deps init uninstall init-db empty-db build

BIN_NAME	:= nex
DEPS		:= sshpass openssl sqlcipher-devel golang
LOCAL_DIR	:= "/var/lib/nex"
DB3_FILE	:= "$(LOCAL_DIR)/nex.db3"
BASHRC		:= "$(HOME)/.bashrc"
UID 		:= $(shell id -u)

ifeq (,$(filter uninstall deps empty-db,$(MAKECMDGOALS)))
	DECRYPT := $(shell bash -c 'read -s -p "Enter decryption password: " pw; echo $$pw')
endif

all: install init-db

build:
	@rm -f go.mod go.sum ./bin/$(BIN_NAME)
	@go mod init nex >/dev/null 2>&1 && go mod tidy >/dev/null 2>&1
	@CIPHER_KEY=$$(openssl rand -hex 32); \
	CIPHER_TXT=$$(echo $(CIPHER) | base64 -d | openssl aes-256-cbc -d -salt -pbkdf2 -pass pass:$(DECRYPT) 2>/dev/null); \
	if [ -z "$$CIPHER_TXT" ]; then \
		echo "ERROR: Decryption failed. Please ensure you entered the correct password."; \
		exit 1; \
	fi; \
	USER=$$(echo "$$CIPHER_TXT" | awk -F':' '{ print $$1 }'); \
	PASS=$$(echo "$$CIPHER_TXT" | awk -F':' '{ print $$2 }'); \
	FLAGS="-X 'main.DefaultUsername=$$USER' -X 'main.DefaultPassword=$$PASS' -X 'main.SQLCipherKey=$$CIPHER_KEY'"; \
	sudo go build -tags "sqlite_cgo" -ldflags="$$FLAGS" -o ./bin/$(BIN_NAME)
	@echo "[+] Binary built: $$(find ./bin -type f)"

deps: 
	@for pkg in $(DEPS) ; do if ! command -v $$pkg >/dev/null ; then sudo dnf install $$pkg ; fi ; done

init: 
	@if [ $(UID) -eq 0 ] ; then echo -e "\n[E] Do not run as root!" ; exit 1 ; fi  
	@sudo mkdir -p $(LOCAL_DIR)
	@sudo install -m 755 -v ./autocomplete.sh $(LOCAL_DIR)/autocomplete.sh
	-@if ! grep -q nex.*autocomplete.sh $(BASHRC) ; then \
		sudo bash -c 'echo "source $(LOCAL_DIR)/autocomplete.sh" >> $(BASHRC)'; \
	fi
	-@if ! grep nex=\'sudo\ nex $(HOME)/.bash_aliases ; then \
		sudo bash -c "echo  alias nex=\'sudo nex\' >> $(HOME)/.bash_aliases"; \
	fi  
	@source $(BASHRC); \
	
install: init deps build 
	@sudo install -m 755 -v ./bin/nex /bin
	@echo "[+] Building Database" ; nex install | tail -1
	@sudo nex dump-db
	@echo "COMPLETE"
	@echo "For autocomplete functionality, you must run: source $(BASHRC)"

empty-db: 
	@sudo rm $(DB3_FILE)
	@touch empty.csv && sudo nex build-db empty.csv | grep -v csv && rm empty.csv
	@sudo nex dump-db

uninstall: 
	sudo rm -f /bin/nex
	sudo rm -rf $(LOCAL_DIR)
	sudo sed -i "/nex.*autocomplete/d" $(BASHRC)
