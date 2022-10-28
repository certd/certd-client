import { request } from "/src/api/service";
import _ from "lodash-es";
import dayjs from "dayjs";
const apiPrefix = "/pi/history";

export async function GetList(query) {
  const list = await request({
    url: apiPrefix + "/list",
    method: "post",
    data: query
  });
  for (const item of list) {
    if (item.results) {
      item.results = JSON.parse(item.results);
    }
    if (item.logs) {
      item.logs = JSON.parse(item.logs);
    }
  }
  console.log("history", list);
  return list;
}
