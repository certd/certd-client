import * as api from "./api";
<<<<<<<< HEAD:src/views/crud/feature/remove/crud.tsx
import { useI18n } from "vue-i18n";
import { ref } from "vue";
import { getCommonColumnDefine } from "/@/views/certd/access/common";

========
import { dict } from "@fast-crud/fast-crud";
import { ref } from "vue";
>>>>>>>> upstream/main:src/views/crud/basis/columns-set/crud.jsx
export default function ({ expose }) {
  const { t } = useI18n();
  const pageRequest = async (query) => {
    return await api.GetList(query);
  };
  const editRequest = async ({ form, row }) => {
    form.id = row.id;
    return await api.UpdateObj(form);
  };
  const delRequest = async ({ row }) => {
    return await api.DelObj(row.id);
  };

  const addRequest = async ({ form }) => {
    return await api.AddObj(form);
  };
  const typeRef = ref();
  const { crudBinding } = expose;
  const commonColumnsDefine = getCommonColumnDefine(crudBinding, typeRef);
  return {
    crudOptions: {
      request: {
        pageRequest,
        addRequest,
        editRequest,
        delRequest
      },
<<<<<<<< HEAD:src/views/crud/feature/remove/crud.tsx
      form: {
        labelCol: {
          span: 6
========
      toolbar: {
        columnsFilter: {
          mode: "default"
>>>>>>>> upstream/main:src/views/crud/basis/columns-set/crud.jsx
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
<<<<<<<< HEAD:src/views/crud/feature/remove/crud.tsx
        name: {
          title: "名称",
          type: "text",
          form: {
            rules: [{ required: true, message: "必填项" }]
========
        radio: {
          title: "状态",
          search: { show: true },
          type: "dict-radio",
          dict: dict({
            url: "/mock/dicts/OpenStatusEnum?single"
          })
        },
        disabled: {
          title: "列设置禁用",
          type: "text",
          column: {
            columnSetDisabled: true
          }
        },
        hidden: {
          title: "列设置隐藏",
          type: "text",
          column: {
            columnSetShow: false
>>>>>>>> upstream/main:src/views/crud/basis/columns-set/crud.jsx
          }
        },
        ...commonColumnsDefine
      }
    }
  };
}
