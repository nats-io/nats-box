alias stan-sub="stan_sub"
alias stan-pub="stan_pub"
alias nats-pub="nats_pub"
alias nats-sub="nats_sub"
alias nats-req="nats_req"
alias nats-rply="nats_rply"
alias nats-top="nats_top"

function stan_sub() {
	if [ -n "$STAN_CREDS" ] && [ -n "$STAN_CLUSTER" ] && [ -n "$NATS_URL" ]; then
		/usr/local/bin/stan-sub -creds $STAN_CREDS -c $STAN_CLUSTER -s $NATS_URL "$@"
	elif [ -n "$STAN_CLUSTER" ] && [ -n "$NATS_URL" ]; then
		/usr/local/bin/stan-sub -c $STAN_CLUSTER -s $NATS_URL "$@"
	else
		/usr/local/bin/stan-sub "$@"
	fi
}

function stan_pub() {
	if [[ -n "$STAN_CREDS" ]] && [[ -n "$STAN_CLUSTER" ]] && [[ -n "$NATS_URL" ]]; then
		/usr/local/bin/stan-pub -creds $STAN_CREDS -c $STAN_CLUSTER -s $NATS_URL "$@"
	elif [ -n "$STAN_CLUSTER" ] && [ -n "$NATS_URL" ]; then
		/usr/local/bin/stan-pub -c $STAN_CLUSTER -s $NATS_URL "$@"
	else
		/usr/local/bin/stan-pub "$@"
	fi
}

function nats_pub() {
	if [[ -n "$USER_CREDS" ]]; then
		/usr/local/bin/nats-pub -creds $USER_CREDS "$@"
	else
		/usr/local/bin/nats-pub "$@"
	fi
}

function nats_sub() {
	if [[ -n "$USER_CREDS" ]]; then
		/usr/local/bin/nats-sub -creds $USER_CREDS "$@"
	else
		/usr/local/bin/nats-sub "$@"
	fi
}

function nats_req() {
	if [ -n "$USER2_CREDS" ]; then
		/usr/local/bin/nats-req -creds $USER2_CREDS "$@"
	else
		/usr/local/bin/nats-req "$@"
	fi
}

function nats_rply() {
	if [ -n "$USER_CREDS" ]; then
		/usr/local/bin/nats-rply -q 'nats-box' -creds $USER_CREDS "$@"
	else
		/usr/local/bin/nats-rply -q 'nats-box' "$@"
	fi
}

function nats_top() {
	if [ -n "$NATS_URL" ]; then
		/usr/local/bin/nats-top -s $NATS_URL "$@"
	else
		/usr/local/bin/nats-top "$@"
	fi
}

figlet -p "nats-box"
/usr/local/bin/nats-pub -v
