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

function "get_platforms_multiarch" {
  params = []
  result = CI ? ["linux/amd64", "linux/arm/v6", "linux/arm/v7", "linux/arm64"] : []
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
    "nats-box"
  ]
}

###################
### Targets
###################

target "nats-box" {
  dockerfile = "Dockerfile"
  args = {
    VERSION_NATS        = "0.0.35"
    VERSION_NATS_TOP    = "0.6.1"
    VERSION_NSC         = "2.8.0"
    VERSION_STAN        = "0.10.4"
  }
  platforms  = get_platforms_multiarch()
  tags       = get_tags("nats-box")
  output     = get_output()
}
