export const certdResources = [
  {
    title: "证书管理",
    name: "certd",
    path: "/certd",
    redirect: "/certd/cert",
    meta: {
      icon: "ion:key-outline"
    },
    children: [
      {
        title: "证书管理",
        name: "CertdCert",
        path: "/certd/cert",
        component: "/certd/cert/index.vue",
        meta: {
          icon: "ion:ribbon-outline"
        },
        children: [
          {
            title: "证书详情",
            name: "CertdCertDetail",
            path: "/certd/cert/detail",
            component: "/certd/cert/detail.vue",
            meta: {
              isMenu: false
            }
          }
        ]
      },
      {
        title: "授权",
        name: "access",
        path: "/certd/access",
        component: "/certd/access/index.vue",
        meta: {
          icon: "ion:disc-outline"
        }
      }
    ]
  }
];
