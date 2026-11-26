data "ramble_resource" "mysql" {
  namespace = "alice"
  name      = "mysql"
}

output "mysql_latest_version" {
  value = data.ramble_resource.mysql.latest_version
}
