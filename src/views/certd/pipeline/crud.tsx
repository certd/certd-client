import * as api from "./api";
import { useI18n } from "vue-i18n";
import { ref, shallowRef } from "vue";
import { useRouter } from "vue-router";
export default function ({ expose }) {
  const router = useRouter();
  const { t } = useI18n();
  const lastResRef = ref();
  const pageRequest = async (query) => {
    return await api.GetList(query);
  };
  const editRequest = async ({ form, row }) => {
    form.id = row.id;
    const res = await api.UpdateObj(form);
    lastResRef.value = res;
    return res;
  };
  const delRequest = async ({ row }) => {
    return await api.DelObj(row.id);
  };

  const addRequest = async ({ form }) => {
    const res = await api.AddObj(form);
    lastResRef.value = res;
    return res;
  };
  return {
    crudOptions: {
      request: {
        pageRequest,
        addRequest,
        editRequest,
        delRequest
      },
      rowHandle: {
        buttons: {
          view: {
            click({ row }) {
              router.push({ path: "/certd/pipeline/detail", query: { id: row.id, editMode: "false" } });
            }
          },
          edit: {
            click({ row }) {
              router.push({ path: "/certd/pipeline/detail", query: { id: row.id, editMode: "true" } });
            }
          }
        }
      },
      columns: {
        id: {
          title: "ID",
          key: "id",
          type: "number",
          column: {
            width: 50
          },
          form: {
            show: false
          }
        },
        title: {
          title: "流水线名称",
          type: "text",
          search: {
            show: true,
            component: {
              name: "a-input"
            }
          }
        },
        remark: {
          title: "备注",
          type: "text"
        },
        status: {
          title: "状态",
          type: "text"
        },
        createTime: {
          title: "创建时间",
          type: "datetime"
        },
        updateTime: {
          title: "更新时间",
          type: "datetime"
        }
      }
    }
  };
}
