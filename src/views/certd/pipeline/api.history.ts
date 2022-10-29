import { request } from "/src/api/service";

const apiPrefix = "/pi/history";

export async function GetList(query) {
  const list = await request({
    url: apiPrefix + "/list",
    method: "post",
    data: query
  });
  for (const item of list) {
    if (item.pipeline) {
      item.pipeline = JSON.parse(item.pipeline);
    }
  }
  console.log("history", list);
  return list;
}

export async function GetLogs(historyId: any) {
  const log = await request({
    url: apiPrefix + "/logs",
    method: "post",
    params: { id: historyId }
  });

  if (log) {
    log.logs = JSON.parse(log?.logs || "{}");
    return log;
  } else {
    return { logs: {} };
  }
}
