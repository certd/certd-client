import * as api from "./api";
import { compute, dict } from "@fast-crud/fast-crud";
import { ref, watch } from "vue";
import { useRouter } from "vue-router";
export default function ({ expose, props, ctx }) {
  const lastResRef = ref();
  const pageRequest = async (query) => {
    return await api.GetList(query);
  };
  const editRequest = async ({ form, row }) => {
    form.id = row.id;
    form.type = props.type;
    const res = await api.UpdateObj(form);
    lastResRef.value = res;
    return res;
  };
  const delRequest = async ({ row }) => {
    return await api.DelObj(row.id);
  };

  const addRequest = async ({ form }) => {
    form.type = props.type;
    const res = await api.AddObj(form);
    lastResRef.value = res;
    return res;
  };
  let DNSProviderTypeDictRef = dict({
    data: [
      { value: "aliyun", label: "Aliyun" },
      { value: "dnspod", label: "DnsPod" }
    ]
  });
  const selectedRowKey = ref([props.modelValue]);
  // watch(
  //   () => {
  //     return props.modelValue;
  //   },
  //   (value) => {
  //     selectedRowKey.value = [value];
  //   },
  //   {
  //     immediate: true
  //   }
  // );
  const onSelectChange = (changed) => {
    selectedRowKey.value = changed;
    ctx.emit("update:modelValue", changed[0]);
  };
  return {
    crudOptions: {
      request: {
        pageRequest,
        addRequest,
        editRequest,
        delRequest
      },
      toolbar: {
        show: false
      },
      form: {
        wrapper: {
          width: "1050px"
        }
      },
      rowHandle: {
        width: "150px"
      },
      table: {
        rowSelection: {
          type: "radio",
          selectedRowKeys: selectedRowKey,
          onChange: onSelectChange
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
        name: {
          title: "名称",
          search: {
            show: true
          },
          type: ["text"],
          form: {
            rules: [{ required: true, message: "请填写名称" }]
          }
        },
        type: {
          title: "类型",
          type: ["text"],
          dict: DNSProviderTypeDictRef,
          form: {
            rules: [{ required: true, message: "请选择类型" }]
          }
        },
        settings: {
          title: "配置",
          type: "text",
          column: {
            show: false
          },
          form: {
            rules: [{ required: true, message: "请填写配置" }]
          }
        }
      }
    }
  };
}
