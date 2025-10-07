###################
### Variables
###################

variable REGISTRY {
  default = ""
}

# Comma delimited list of tags
variable TAGS {
  default = "latest"
}

variable CI {
  default = false
}

###################
### Functions
###################

function "get_tags" {
  params = [image]
  result = [for tag in split(",", TAGS) : join("/", compact([REGISTRY, "${image}:${tag}"]))]
}

function "get_tags_suffix" {
  params = [image, suffix]
  result = [for tag in split(",", TAGS) : join("/", compact([REGISTRY, replace("${image}:${tag}-${suffix}", "latest-", "")]))]
}

function "get_platforms_multiarch" {
  params = []
  result = CI ? ["linux/amd64", "linux/s390x", "linux/arm/v6", "linux/arm/v7", "linux/arm64"] : []
}

function "get_output" {
  params = []
  result = CI ? ["type=registry"] : ["type=docker"]
}

###################
### Groups
###################

group "default" {
  targets = [
    "nats-box",
    "nats-box-nonroot"
  ]
}

###################
### Targets
###################

target "nats-box" {
  dockerfile = "Dockerfile"
  args = {
    VERSION_NATS        = "0.3.0"
    VERSION_NATS_TOP    = "0.6.3"
    VERSION_NSC         = "2.12.0"
  }
  platforms  = get_platforms_multiarch()
  tags       = get_tags("nats-box")
  output     = get_output()
}

target "nats-box-nonroot" {
  contexts = {
    nats-box = "target:nats-box"
  }
  inherits = ["nats-box"]
  args = {
    USER = "1000"
  }
  dockerfile-inline = <<EOT
FROM nats-box
ARG USER
USER $USER:$USER
WORKDIR /home/nats
EOT

  tags = get_tags_suffix("nats-box", "nonroot")
}
