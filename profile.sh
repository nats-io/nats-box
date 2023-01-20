alias stan-sub="stan_sub"
alias stan-pub="stan_pub"
alias stan-bench="stan_bench"
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

function stan_bench() {
	if [[ -n "$STAN_CREDS" ]] && [[ -n "$STAN_CLUSTER" ]] && [[ -n "$NATS_URL" ]]; then
		/usr/local/bin/stan-bench -creds $STAN_CREDS -c $STAN_CLUSTER -s $NATS_URL "$@"
	elif [ -n "$STAN_CLUSTER" ] && [ -n "$NATS_URL" ]; then
		/usr/local/bin/stan-bench -c $STAN_CLUSTER -s $NATS_URL "$@"
	else
		/usr/local/bin/stan-bench "$@"
	fi
}

function nats_top() {
	if [ -n "$NATS_URL" ]; then
		/usr/local/bin/nats-top -s $NATS_URL "$@"
	else
		/usr/local/bin/nats-top "$@"
	fi
}

figlet -p "nats-box" >&2
echo "nats-box v0.13.3" >&2
