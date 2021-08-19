export GO111MODULE=on
DB_HOST:=127.0.0.1
DB_PORT:=3306
DB_USER:=isucon
DB_PASS:=isucon
DB_NAME:=isucon2021_prior
MYSQL_CMD:=mysql -h$(DB_HOST) -P$(DB_PORT) -u$(DB_USER) -p$(DB_PASS) $(DB_NAME)

NGX_LOG:=/var/log/nginx/access.log
MYSQL_LOG:=/tmp/slow-query.log


#--------------------
# 事前準備
#--------------------

.PHONY: setup
setup:
	sudo apt install -y percona-toolkit git unzip
	git init
	git config --global user.name karamaru-alpha
	git config --global user.email mrnk3078@gmail.com
	git config credential.helper store
	wget https://github.com/matsuu/kataribe/releases/download/v0.4.1/kataribe-v0.4.1_linux_amd64.zip -O kataribe.zip
	unzip -o kataribe.zip
	sudo mv kataribe /usr/local/bin/
	sudo chmod +x /usr/local/bin/kataribe
	sudo rm kataribe.zip
	kataribe -generate


#--------------------
# ベンチマーカー周り
#--------------------

.PHONY: before
before:
	$(eval when := $(shell date "+%s"))
	mkdir -p ~/logs/$(when)
	@if [ -f $(NGX_LOG) ]; then \
			sudo mv -f $(NGX_LOG) ~/logs/$(when)/ ; \
	fi
	@if [ -f $(MYSQL_LOG) ]; then \
			sudo mv -f $(MYSQL_LOG) ~/logs/$(when)/ ; \
	fi
	sudo cp nginx.conf /etc/nginx/nginx.conf
	sudo cp my.cnf /etc/mysql/my.cnf
# (cd go && make) -> build
	sudo systemctl restart nginx
	sudo systemctl restart mysql
#	sudo systemctl restart isucari.golang.service



.PHONY: bench
bench:
	../bin/benchmarker -target 127.0.0.1:8080

#--------------------
# mysql周り cnfならいらない
#--------------------

.PHONY: sql
sql:
	mysql -h$(DB_HOST) -P$(DB_PORT) -u$(DB_USER) -p$(DB_PASS) $(DB_NAME)

.PHONY: slow
slow:
	sudo pt-query-digest $(MYSQL_LOG)

# 再起動すると設定リセットされるので注意
.PHONY: slow-on
slow-on:
	sudo mysql -e "set global slow_query_log_file = '$(MYSQL_LOG)'; set global long_query_time = 0; set global slow_query_log = ON;"
# sudo $(MYSQL_CMD) -e "set global slow_query_log_file = '$(MYSQL_LOG)'; set global long_query_time = 0; set global slow_query_log = ON;"

.PHONY: slow-off
slow-off:
	sudo mysql -e "set global slow_query_log = OFF;"
# sudo $(MYSQL_CMD) -e "set global slow_query_log = OFF;"


#--------------------
# accesslog周り
#--------------------

.PHONY: kataru
kataru:
	sudo cat $(NGX_LOG) | kataribe -f ./kataribe.toml
