<template>
  <fs-page class="page-cert-detail">
    <div v-if="detailRef.cert" class="cert-detail">
      <div class="cert-detail-left">
        <a-page-header title="证书申请" sub-title="可以将多个域名打到一个证书上">
          <template #extra>
            <a-button key="apply" type="primary">立即申请</a-button>
            <a-button key="edit" type="primary" @click="openCertEditDialog">编辑证书</a-button>
          </template>
          <a-descriptions bordered>
            <a-descriptions-item label="域名"
              ><certd-domains :value="detailRef.cert.domains"></certd-domains
            ></a-descriptions-item>
            <a-descriptions-item label="邮箱">{{ detailRef.cert.email }}</a-descriptions-item>
            <a-descriptions-item label="证书签发者">
              <fs-values-format v-model="detailRef.cert.certIssuerId" :dict="certIssuerDict"></fs-values-format>
            </a-descriptions-item>
            <a-descriptions-item label="DNS提供商">
              <fs-values-format
                v-model="detailRef.cert.challengeDnsType"
                :dict="dnsProviderTypeDict"
              ></fs-values-format>
            </a-descriptions-item>
            <a-descriptions-item label="DNS授权">
              <label-show v-model="detailRef.cert.challengeAccessId" label-key="name" :get-info="getAccessInfo">
              </label-show>
            </a-descriptions-item>
          </a-descriptions>

          <fs-form-wrapper ref="certEditDialogRef" v-bind="certEditDialogOptions" />
        </a-page-header>
        <cert-deploy ref="certDeployRef" v-model="detailRef.deploy"></cert-deploy>
      </div>

      <div class="cert-log"></div>
    </div>
  </fs-page>
</template>

<script>
import { defineComponent, ref, onMounted, computed } from "vue";
import { useRoute } from "vue-router";
import CertdDomains from "./domains.vue";
import { useColumns } from "@fast-crud/fast-crud";
import createCrudOptions from "./crud";
import * as api from "./api";
import { Dicts } from "./dicts";
import LabelShow from "./components/label-show.vue";
import CertDeploy from "./components/cert-deploy.vue";
function createFormOptionsFromCrudOptions() {
  const { buildFormOptions } = useColumns();
  //可以直接复用crud.js
  const { crudOptions } = createCrudOptions({});
  return buildFormOptions(crudOptions);
}

function useCertEditDialog() {
  const certEditDialogRef = ref();
  const certEditDialogOptions = ref(createFormOptionsFromCrudOptions());
  function openCertEditDialog() {
    certEditDialogRef.value.open(certEditDialogOptions.value);
  }
  return {
    certEditDialogRef,
    openCertEditDialog,
    certEditDialogOptions
  };
}

export default defineComponent({
  name: "CertdCertDetail",
  components: { CertdDomains, LabelShow, CertDeploy },
  setup() {
    const route = useRoute();
    const id = route.query.id;
    const detailRef = ref({});

    let certEditDialogProps = useCertEditDialog();

    async function getDetail(id) {
      const detail = await api.GetDetail(id);
      detailRef.value = detail;
      certEditDialogProps.certEditDialogOptions.value.initialForm = detail.cert;
    }
    getDetail(id);

    const getAccessInfo = api.GetAccessInfo;

    const certDeployRef = ref();
    return {
      detailRef,
      certIssuerDict: Dicts.certIssuerDict,
      challengeTypeDict: Dicts.challengeTypeDict,
      dnsProviderTypeDict: Dicts.dnsProviderTypeDict,
      getAccessInfo,
      certDeployRef,
      ...certEditDialogProps
    };
  }
});
</script>
<style lang="less">
.page-cert-detail {
  height: 100%;

  .ant-page-header-heading-title {
    font-size: 16px;
  }
  .ant-page-header-heading-sub-title {
    font-size: 12px;
    line-height: initial;
  }
  .content-box {
    padding: 20px;
  }
  .cert-detail {
    display: flex;
    .cert-detail-left {
      width: 70%;
    }
    .cert-log {
      width: 30%;
    }
  }
}
</style>
