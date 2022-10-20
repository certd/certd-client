import { request } from "/src/api/service";
const apiPrefix = "/certd/plugin";
export function GetList(query) {
  return request({
    url: apiPrefix + "/list",
    method: "post",
    params: query
  });
}
