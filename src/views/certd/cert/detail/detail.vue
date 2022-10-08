<template>
  <fs-page class="page-cert-detail">
    <template #header>
      <div class="title">证书详情</div>
    </template>
    <div v-if="detailRef" class="cert-detail">
      <div class="cert-detail-left">
        <a-card class="cert-info" title="证书信息">
          <a-descriptions>
            <a-descriptions-item label="域名"
              ><certd-domains :value="detailRef.cert.domains"></certd-domains
            ></a-descriptions-item>
            <a-descriptions-item label="邮箱">1810000000</a-descriptions-item>
            <a-descriptions-item label="其它信息">Hangzhou, Zhejiang</a-descriptions-item>
          </a-descriptions>
        </a-card>

        <div class="cert-deploy">
          <a-card title="自动部署">
            <a-list
              :grid="{ gutter: 16, xs: 1, sm: 2, md: 3, lg: 3, xl: 4, xxl: 4, xxxl: 6 }"
              :data-source="detailRef.deploy"
            >
              <template #renderItem="{ item }">
                <a-list-item>
                  <a-card hoverable>
                    <template #title>
                      {{ item.name }}
                    </template>
                    <template #extra> <a-tag color="green">成功</a-tag> </template>
                  </a-card>
                </a-list-item>
              </template>
            </a-list>
          </a-card>
        </div>
      </div>

      <div class="cert-log"></div>
    </div>
  </fs-page>
</template>

<script>
import { defineComponent, ref, onMounted, computed } from "vue";
import { useRoute } from "vue-router";
import CertdDomains from "/src/views/certd/cert/domains.vue";
import * as api from "../api";
export default defineComponent({
  name: "CertdCertDetail",
  components: { CertdDomains },
  setup() {
    const route = useRoute();
    const id = route.query.id;
    const detailRef = ref();
    async function getDetail(id) {
      // const detail = await api.GetDetail(id);
      const detail = {
        cert: { domains: "*.yonsz.com,test.yonsz.net" },
        deploy: [
          {
            id: "xxxxx",
            name: "部署到阿里云AckIngress",
            type: "AliyunAckIngress"
          }
        ],
        lastHistory: {
          result: "success",
          log: "xxxx"
        }
      };
      detailRef.value = detail;
    }
    getDetail(id);
    return {
      detailRef
    };
  }
});
</script>
<style lang="less">
.page-cert-detail {
  .cert-deploy {
    margin-top: 20px;
  }
  .cert-detail {
    padding: 20px;
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
