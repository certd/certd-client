import * as api from "./api";
import { compute, dict } from "@fast-crud/fast-crud";
import { useI18n } from "vue-i18n";
import AccessSelector from "./components/access-selector.vue";
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
  let DNSProviderTypeDictRef = dict({
    data: [
      { value: "aliyun", label: "Aliyun" },
      { value: "dnspod", label: "DnsPod" }
    ]
  });
  return {
    crudOptions: {
      request: {
        pageRequest,
        addRequest,
        editRequest,
        delRequest
      },
      table: {
        show: false
      },
      toolbar: {
        show: false
      },
      form: {
        wrapper: {
          width: "1050px"
        },
        afterSubmit({ mode, form }) {
          if (mode === "add") {
            //跳转到详情页
            console.log("add success:", form);
            router.push({ path: "cert/detail", query: { id: lastResRef.value.id } });
          }
        },
        group: {
          type: "collapse", // tab
          accordion: false, //手风琴模式
          groups: {
            base: {
              header: "证书基本信息",
              columns: [
                "domains",
                "email",
                "certIssuerId",
                "challengeType",
                "challengeDnsType",
                "challengeAccessId",
                "remark"
              ]
            },
            info: {
              header: "证书CSR信息",
              slots: {
                extra: () => {
                  return <span style={"font-size:12px;color:red"}>只能填英文</span>;
                }
              },
              columns: ["country", "state", "locality", "organization", "organizationUnit"]
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
        domains: {
          title: "域名",
          type: ["dict-select", "colspan"],
          search: {
            show: true,
            component: {
              name: "a-input"
            }
          },
          form: {
            component: {
              mode: "tags",
              open: false
            },
            helper: {
              render: () => {
                return (
                  <div>
                    <div>支持通配符域名，例如： *.foo.com 、 *.test.handsfree.work</div>
                    <div>支持多个域名、多个子域名、多个通配符域名打到一个证书上（域名必须是在同一个DNS提供商解析）</div>
                    <div>多级子域名要分成多个域名输入（*.foo.com的证书不能用于xxx.yyy.foo.com）</div>
                    <div>输入一个回车之后，再输入下一个</div>
                  </div>
                );
              }
            },
            valueResolve({ form }) {
              form.domains = form.domains?.join(",");
            },
            rules: [{ required: true, message: "请填写域名" }]
          }
        },
        email: {
          title: "邮箱",
          type: "text",
          search: { show: false },
          form: {
            rules: [{ required: true, type: "email", message: "请填写邮箱" }]
          }
        },
        certIssuerId: {
          title: "证书签发者",
          type: "dict-select",
          dict: dict({ data: [{ value: "letencrypt", label: "LetEncrypt" }] }),
          form: {
            value: "letencrypt",
            rules: [{ required: true, message: "请填写域名" }]
          }
        },
        challengeType: {
          title: "校验方式",
          type: "dict-select",
          dict: dict({ data: [{ value: "dns", label: "DNS校验" }] }),
          form: {
            value: "dns",
            rules: [{ required: true, message: "请填写域名" }]
          }
        },
        challengeDnsType: {
          title: "DNS提供商",
          type: "dict-select",
          dict: DNSProviderTypeDictRef,
          form: {
            value: "aliyun",
            rules: [{ required: true, message: "请选择DNS提供商" }]
          }
        },
        challengeAccessId: {
          title: "DNS授权",
          type: "text",
          form: {
            component: {
              name: shallowRef(AccessSelector),
              type: compute(({ form }) => {
                return form.challengeDnsType;
              }),
              vModel: "modelValue"
            },
            rules: [{ required: true, message: "请选择DNS授权" }]
          }
        },
        country: {
          title: "国家",
          type: "text",
          form: {
            value: "China"
          }
        },
        state: {
          title: "省份",
          type: "text",
          form: {
            value: "GuangDong"
          }
        },
        locality: {
          title: "市区",
          type: "text",
          form: {
            value: "NanShan"
          }
        },
        organization: {
          title: "单位",
          type: "text",
          form: {
            value: "CertD"
          }
        },
        organizationUnit: {
          title: "部门",
          type: "text",
          form: {
            value: "IT Dept"
          }
        },
        remark: {
          title: "备注",
          type: "text"
        }
      }
    }
  };
}
