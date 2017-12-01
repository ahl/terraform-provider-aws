package aws

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/apigateway"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceAwsApiGatewayVpcLink() *schema.Resource {
	return &schema.Resource{
		Create: resourceAwsApiGatewayVpcLinkCreate,
		Read:   resourceAwsApiGatewayVpcLinkRead,
		Update: resourceAwsApiGatewayVpcLinkUpdate,
		Delete: resourceAwsApiGatewayVpcLinkDelete,

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"description": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"target_arns": {
				Type:     schema.TypeSet,
				Required: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Set:      schema.HashString,
			},
		},
	}
}

func resourceAwsApiGatewayVpcLinkCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).apigateway

	input := &apigateway.CreateVpcLinkInput{
		Name:       aws.String(d.Get("name").(string)),
		TargetArns: expandStringSet(d.Get("target_arns").(*schema.Set)),
	}
	if v, ok := d.GetOk("description"); ok {
		input.Description = aws.String(v.(string))
	}

	resp, err := conn.CreateVpcLink(input)
	if err != nil {
		return err
	}

	stateConf := &resource.StateChangeConf{
		Pending:    []string{apigateway.VpcLinkStatusPending},
		Target:     []string{apigateway.VpcLinkStatusAvailable},
		Refresh:    apigatewayVpcLinkRefreshStatusFunc(conn, *resp.Id),
		Timeout:    10 * time.Minute,
		Delay:      10 * time.Second,
		MinTimeout: 3 * time.Second,
	}

	_, err = stateConf.WaitForState()
	if err != nil {
		return fmt.Errorf("[WARN] Error waiting for APIGateway Vpc Link status to be \"%s\": %s", apigateway.VpcLinkStatusAvailable, err)
	}

	d.SetId(*resp.Id)
	return nil
}

func resourceAwsApiGatewayVpcLinkRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).apigateway

	input := &apigateway.GetVpcLinkInput{
		VpcLinkId: aws.String(d.Id()),
	}

	resp, err := conn.GetVpcLink(input)
	if err != nil {
		if ecrerr, ok := err.(awserr.Error); ok {
			switch ecrerr.Code() {
			case apigateway.ErrCodeNotFoundException:
				d.SetId("")
				return nil
			}
		}
		return err
	}

	d.Set("name", resp.Name)
	d.Set("description", resp.Description)
	d.Set("target_arns", schema.NewSet(schema.HashString, flattenStringList(resp.TargetArns)))
	return nil
}

func resourceAwsApiGatewayVpcLinkUpdate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).apigateway

	operations := make([]*apigateway.PatchOperation, 0)

	if d.HasChange("name") {
		operations = append(operations, &apigateway.PatchOperation{
			Op:    aws.String("replace"),
			Path:  aws.String("/name"),
			Value: aws.String(d.Get("name").(string)),
		})
	}

	if d.HasChange("description") {
		operations = append(operations, &apigateway.PatchOperation{
			Op:    aws.String("replace"),
			Path:  aws.String("/description"),
			Value: aws.String(d.Get("description").(string)),
		})
	}

	if d.HasChange("target_arns") {
		o, n := d.GetChange("target_arns")
		prefix := "targetArns"

		os := o.(*schema.Set)
		ns := n.(*schema.Set)
		adddiff := ns.Difference(os).List()
		remdiff := os.Difference(ns).List()

		for _, v := range remdiff {
			operations = append(operations, &apigateway.PatchOperation{
				Op:   aws.String("remove"),
				Path: aws.String(fmt.Sprintf("/%s/%s", prefix, escapeJsonPointer(v.(string)))),
			})
		}

		for _, v := range adddiff {
			operations = append(operations, &apigateway.PatchOperation{
				Op:   aws.String("add"),
				Path: aws.String(fmt.Sprintf("/%s/%s", prefix, escapeJsonPointer(v.(string)))),
			})
		}
	}

	input := &apigateway.UpdateVpcLinkInput{
		VpcLinkId:       aws.String(d.Id()),
		PatchOperations: operations,
	}

	_, err := conn.UpdateVpcLink(input)
	if err != nil {
		if ecrerr, ok := err.(awserr.Error); ok {
			switch ecrerr.Code() {
			case apigateway.ErrCodeNotFoundException:
				d.SetId("")
				return nil
			}
		}
		return err
	}

	stateConf := &resource.StateChangeConf{
		Pending:    []string{apigateway.VpcLinkStatusPending},
		Target:     []string{apigateway.VpcLinkStatusAvailable},
		Refresh:    apigatewayVpcLinkRefreshStatusFunc(conn, d.Id()),
		Timeout:    10 * time.Minute,
		Delay:      10 * time.Second,
		MinTimeout: 3 * time.Second,
	}

	_, err = stateConf.WaitForState()
	if err != nil {
		return fmt.Errorf("[WARN] Error waiting for APIGateway Vpc Link status to be \"%s\": %s", apigateway.VpcLinkStatusAvailable, err)
	}

	return nil
}

func resourceAwsApiGatewayVpcLinkDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).apigateway

	input := &apigateway.DeleteVpcLinkInput{
		VpcLinkId: aws.String(d.Id()),
	}

	_, err := conn.DeleteVpcLink(input)
	if err != nil {
		if ecrerr, ok := err.(awserr.Error); ok {
			switch ecrerr.Code() {
			case apigateway.ErrCodeNotFoundException:
				d.SetId("")
				return nil
			}
		}
		return err
	}

	d.SetId("")
	return nil
}

func apigatewayVpcLinkRefreshStatusFunc(conn *apigateway.APIGateway, vl string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		input := &apigateway.GetVpcLinkInput{
			VpcLinkId: aws.String(vl),
		}
		resp, err := conn.GetVpcLink(input)
		if err != nil {
			return nil, "failed", err
		}
		return resp, *resp.Status, nil
	}
}