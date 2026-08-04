variable "database_url" {
  type    = string
  default = "postgres://postgres:postgres@localhost:5432/dugble?sslmode=disable&search_path=public"
}

variable "dev_database_url" {
  type    = string
  default = "postgres://postgres:postgres@localhost:5432/dugble_atlas_dev?sslmode=disable&search_path=public"
}

env "local" {
  url = var.database_url
  dev = var.dev_database_url

  migration {
    dir = "file://migrations"
  }
}